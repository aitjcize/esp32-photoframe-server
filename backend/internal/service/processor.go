package service

import (
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/photoframe"
	_ "golang.org/x/image/bmp" // Register BMP decoder
)

type ProcessorService struct {
}

func NewProcessorService() *ProcessorService {
	return &ProcessorService{}
}

func (s *ProcessorService) MapProcessingSettings(settings *photoframe.ProcessingSettings, palette *photoframe.Palette, grayscale bool) map[string]string {
	opts := make(map[string]string)

	// Tone/dither options come from the device's stored settings. When settings
	// are absent (e.g. a push to a device that never saved any), emit none and
	// let the CLI use its sane defaults -- emitting zeros here would, for
	// example, force exposure/contrast to 0 and render a black image. The
	// palette below is still emitted so a grayscale panel dithers to gray.
	if settings != nil {
		opts["exposure"] = fmt.Sprintf("%v", settings.Exposure)
		opts["saturation"] = fmt.Sprintf("%v", settings.Saturation)
		if settings.ToneMode != "" {
			opts["tone-mode"] = settings.ToneMode
		}
		opts["contrast"] = fmt.Sprintf("%v", settings.Contrast)
		if settings.ToneMode == "scurve" {
			opts["scurve-strength"] = fmt.Sprintf("%v", settings.Strength)
			opts["scurve-shadow"] = fmt.Sprintf("%v", settings.ShadowBoost)
			opts["scurve-highlight"] = fmt.Sprintf("%v", settings.HighlightCompress)
			opts["scurve-midpoint"] = fmt.Sprintf("%v", settings.Midpoint)
		}
		if settings.ColorMethod != "" {
			opts["color-method"] = settings.ColorMethod
		}
		if settings.DitherAlgorithm != "" {
			opts["dither-algorithm"] = settings.DitherAlgorithm
		}
		if settings.CompressDynamicRange {
			opts["compress-dynamic-range"] = "" // Boolean flag
		}
		// Converter selection: "epaper-image-convert" (default) or "epdoptimize"
		if settings.Converter != "" {
			opts["converter"] = settings.Converter
		}
		if settings.AutoMode {
			opts["auto-mode"] = ""
		}
		if settings.EpdOptimizePreset != "" {
			opts["epd-optimize-preset"] = settings.EpdOptimizePreset
		}
	}

	if grayscale {
		// GC16: the whole gray palette (theoretical output ramp + the calibrated
		// perceived ramp) lives in epaper-image-convert -- we never hardcode a
		// gray ramp here. If the device reported its measured luminance endpoints
		// (Y of full black/white), pass them so the CLI derives the panel's
		// perceived ramp; otherwise fall back to the built-in grayscale16 preset.
		if palette != nil && palette.BlackY != nil && palette.WhiteY != nil {
			opts["gray-black-y"] = fmt.Sprintf("%v", *palette.BlackY)
			opts["gray-white-y"] = fmt.Sprintf("%v", *palette.WhiteY)
			if palette.Gamma != nil {
				opts["gray-gamma"] = fmt.Sprintf("%v", *palette.Gamma)
			}
		} else {
			opts["palette-preset"] = "grayscale16"
		}
	} else if palette != nil {
		paletteWrapper := map[string]interface{}{
			"theoretical": map[string]interface{}{
				"black":  map[string]int{"r": 0, "g": 0, "b": 0},
				"white":  map[string]int{"r": 255, "g": 255, "b": 255},
				"yellow": map[string]int{"r": 255, "g": 255, "b": 0},
				"red":    map[string]int{"r": 255, "g": 0, "b": 0},
				"blue":   map[string]int{"r": 0, "g": 0, "b": 255},
				"green":  map[string]int{"r": 0, "g": 255, "b": 0},
			},
			"perceived": palette,
		}
		paletteJSON, err := json.Marshal(paletteWrapper)
		if err == nil {
			opts["palette"] = string(paletteJSON)
		}
	}

	return opts
}

func (s *ProcessorService) ProcessImage(img image.Image, options map[string]string) ([]byte, []byte, error) {
	// 1. Create temp directory for this operation
	tmpDir, err := os.MkdirTemp("", "process-*")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 2. Save input image
	inputPath := filepath.Join(tmpDir, "source.jpg")
	f, err := os.Create(inputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create input file: %w", err)
	}

	// Encode as JPEG
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 95}); err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("failed to encode input image: %w", err)
	}
	f.Close()

	// 3. Orientation is passed through to CLI as --orientation flag.
	// The CLI swaps dims, processes at oriented dimensions, then rotates
	// output to native panel layout.

	// Prepare output paths
	format := "epdgz"
	if f, ok := options["format"]; ok {
		format = f
		delete(options, "format")
	}

	// Determine converter: "epdoptimize" or "epaper-image-convert" (default)
	converter := "epaper-image-convert"
	if c, ok := options["converter"]; ok && c != "" {
		converter = c
		delete(options, "converter")
	}

	autoMode := false
	if _, ok := options["auto-mode"]; ok {
		autoMode = true
		delete(options, "auto-mode")
	}

	thumbPath := filepath.Join(tmpDir, "thumbnail.jpg")

	if converter == "epdoptimize" {
		return s.processWithEpdOptimize(inputPath, options, format, autoMode, tmpDir, thumbPath)
	}

	return s.processWithEpaperImageConvert(inputPath, options, format, tmpDir, thumbPath)
}

// findWrapperScript locates the epdoptimize wrapper script. It looks relative
// to the current executable first, then falls back to well-known paths.
func findWrapperScript() string {
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "epdoptimize-wrapper.mjs")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Fallback: look relative to working directory
	for _, base := range []string{".", "..", "/app"} {
		candidate := filepath.Join(base, "epdoptimize-wrapper.mjs")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func (s *ProcessorService) processWithEpdOptimize(inputPath string, options map[string]string, format string, autoMode bool, tmpDir string, thumbPath string) ([]byte, []byte, error) {
	wrapperScript := findWrapperScript()
	if wrapperScript == "" {
		return nil, nil, fmt.Errorf("epdoptimize wrapper script not found")
	}

	// The wrapper always outputs PNG; we'll convert to EPDGZ later if needed.
	outputPath := filepath.Join(tmpDir, "output.png")

	// Build settings JSON for the wrapper
	settings := map[string]interface{}{
		"autoMode": autoMode,
	}
	if preset, ok := options["epd-optimize-preset"]; ok {
		settings["preset"] = preset
		delete(options, "epd-optimize-preset")
	}
	if !autoMode {
		// Pass through the processing options from epaper-image-convert
		// and let the wrapper translate them
		settings["exposure"] = options["exposure"]
		settings["saturation"] = options["saturation"]
		settings["contrast"] = options["contrast"]
		settings["toneMode"] = options["tone-mode"]
		settings["ditherAlgorithm"] = options["dither-algorithm"]
		settings["colorMethod"] = options["color-method"]
		settings["compressDynamicRange"] = options["compress-dynamic-range"]
		settings["scurveStrength"] = options["scurve-strength"]
		settings["scurveShadow"] = options["scurve-shadow"]
		settings["scurveHighlight"] = options["scurve-highlight"]
		settings["scurveMidpoint"] = options["scurve-midpoint"]
	}

	settingsJSON, _ := json.Marshal(settings)

	// Determine palette type for the wrapper
	paletteType := "spectra6" // default
	if _, ok := options["palette-preset"]; ok {
		if options["palette-preset"] == "grayscale16" {
			paletteType = "grayscale16"
		}
		delete(options, "palette-preset")
	}
	if _, ok := options["gray-black-y"]; ok {
		paletteType = "grayscale16"
	}

	// Build wrapper arguments
	args := []string{
		wrapperScript,
		inputPath,
		outputPath,
		"-d", options["dimension"],
		"-t", thumbPath,
		"--palette-type", paletteType,
		"--settings", string(settingsJSON),
	}

	if orientation, ok := options["orientation"]; ok {
		args = append(args, "--orientation", orientation)
		delete(options, "orientation")
	}

	// Pass palette data if present (raw palette JSON from epaper-image-convert format)
	if palette, ok := options["palette"]; ok {
		args = append(args, "--palette", palette)
		delete(options, "palette")
	}
	// Pass grayscale calibration if present
	for _, k := range []string{"gray-black-y", "gray-white-y", "gray-gamma"} {
		if v, ok := options[k]; ok {
			args = append(args, "--"+k, v)
			delete(options, k)
		}
	}

	log.Printf("Processing image with epdoptimize: node %s", args[1:])

	nodeBin := "node"
	if runtime.GOOS == "windows" {
		nodeBin = "node.exe"
	}
	cmd := exec.Command(nodeBin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("epdoptimize execution failed: %s\nOutput: %s\n", err, string(output))
		return nil, nil, fmt.Errorf("epdoptimize execution failed: %s", err)
	}
	log.Printf("epdoptimize Output: %s\n", string(output))

	// Read the PNG output from the wrapper
	pngBytes, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read epdoptimize output: %s. Output: %s", err, string(output))
	}

	// If EPDGZ format is needed, convert PNG → EPDGZ via epaper-image-convert
	var processedBytes []byte
	if format == "epdgz" {
		epdgzPath := filepath.Join(tmpDir, "output.epdgz")
		dim := options["dimension"]
		convertArgs := []string{outputPath, epdgzPath, "-f", "epdgz"}
		if dim != "" {
			convertArgs = append(convertArgs, "-d", dim)
		}
		convertArgs = append(convertArgs, "-v")

		log.Printf("Converting PNG to EPDGZ: epaper-image-convert %s", convertArgs)
		convertCmd := exec.Command("epaper-image-convert", convertArgs...)
		convertOutput, err := convertCmd.CombinedOutput()
		if err != nil {
			return nil, nil, fmt.Errorf("EPDGZ conversion failed: %s. Output: %s", err, string(convertOutput))
		}
		log.Printf("EPDGZ conversion output: %s", string(convertOutput))

		processedBytes, err = os.ReadFile(epdgzPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read EPDGZ output: %s", err)
		}
	} else {
		processedBytes = pngBytes
	}

	thumbBytes, err := os.ReadFile(thumbPath)
	if err != nil {
		fmt.Printf("Warning: Thumbnail not generated by epdoptimize wrapper. Path: %s\n", thumbPath)
		thumbBytes = nil
	}

	return processedBytes, thumbBytes, nil
}

func (s *ProcessorService) processWithEpaperImageConvert(inputPath string, options map[string]string, format string, tmpDir string, thumbPath string) ([]byte, []byte, error) {
	// Prepare output paths
	outputExt := format
	if format == "epdgz" {
		outputExt = "epdgz"
	}
	outputPath := filepath.Join(tmpDir, "output."+outputExt)

	// Remove epdoptimize-specific keys that epaper-image-convert doesn't understand
	delete(options, "epd-optimize-preset")

	// Prepare CLI arguments for epaper-image-convert
	// epaper-image-convert input.jpg output.{epdgz,png} -d WxH -f {format} -t thumbnail.jpg [options]
	args := []string{inputPath, outputPath, "-f", format}

	// Add dimension if specified
	if dimension, ok := options["dimension"]; ok {
		args = append(args, "-d", dimension)
	}

	// Add thumbnail output
	args = append(args, "-t", thumbPath)

	// Add other options (excluding dimension which we already handled)
	for k, v := range options {
		if k != "dimension" {
			if v == "" {
				// Boolean flag
				args = append(args, "--"+k)
			} else {
				args = append(args, "--"+k, v)
			}
		}
	}

	// Make verbose
	args = append(args, "-v")
	log.Println("Processing image with arguments: ", args)

	cmd := exec.Command("epaper-image-convert", args...)

	// Capture output for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("CLI execution failed: %s\nOutput: %s\n", err, string(output))
		return nil, nil, fmt.Errorf("cli execution failed: %s", err)
	}

	// Log CLI output for debug
	log.Printf("CLI Output: %s\n", string(output))

	// Read outputs
	processedBytes, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read processed image: %s. CLI Output: %s", err, string(output))
	}

	thumbBytes, err := os.ReadFile(thumbPath)
	if err != nil {
		// If thumbnail missing, maybe acceptable? But CLI should generate it.
		// We'll return nil for thumbBytes if missing, handling it gracefully
		fmt.Printf("Warning: Thumbnail not generated by CLI. Path: %s\n", thumbPath)
		thumbBytes = nil
	} else {
		// fmt.Printf("Processor: Successfully generated thumb (%d bytes)\n", len(thumbBytes))
	}

	return processedBytes, thumbBytes, nil
}
