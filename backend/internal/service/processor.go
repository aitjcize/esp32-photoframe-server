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
	}

	if grayscale {
		// GC16: a 16-level gray ramp (level i -> i*17, since 255/15 == 17).
		// theoretical is the panel's actual output levels; perceived defaults to
		// the same ramp but honors a device-calibrated ramp from X-Color-Palette.
		ramp := make([][]int, 16)
		for i := range ramp {
			v := i * 17
			ramp[i] = []int{v, v, v}
		}
		perceived := ramp
		if palette != nil && len(palette.Grays) > 0 {
			perceived = palette.Grays
		}
		// Emit black/white aliases derived from the ramp ends alongside the
		// grays. The CLI's background + dynamic-range-compression code reads
		// palette.{theoretical,perceived}.black/white; without these aliases an
		// older epaper-image-convert (< 0.1.16) crashes on a bare-grays palette
		// ("Cannot read properties of undefined (reading 'r')"), surfacing as an
		// HTTP 500 when a grayscale frame is served/pushed. Newer CLIs derive the
		// same aliases themselves, so this is a backward-compatible, self-
		// contained palette.
		grayLevel := func(ramp [][]int, i int) map[string]int {
			lvl := ramp[i]
			if len(lvl) < 3 {
				return map[string]int{"r": 0, "g": 0, "b": 0}
			}
			return map[string]int{"r": lvl[0], "g": lvl[1], "b": lvl[2]}
		}
		paletteWrapper := map[string]interface{}{
			"theoretical": map[string]interface{}{
				"grays": ramp,
				"black": grayLevel(ramp, 0),
				"white": grayLevel(ramp, len(ramp)-1),
			},
			"perceived": map[string]interface{}{
				"grays": perceived,
				"black": grayLevel(perceived, 0),
				"white": grayLevel(perceived, len(perceived)-1),
			},
		}
		if paletteJSON, err := json.Marshal(paletteWrapper); err == nil {
			opts["palette"] = string(paletteJSON)
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
	outputExt := format
	if format == "epdgz" {
		outputExt = "epdgz"
	}
	outputPath := filepath.Join(tmpDir, "output."+outputExt)
	thumbPath := filepath.Join(tmpDir, "thumbnail.jpg")

	// 4. Prepare CLI arguments for epaper-image-convert
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

	// 5. Read outputs
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
