package service

import (
	"testing"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/photoframe"
)

// compressDynamicRange must reach the CLI in both states. The CLI's default
// preset (balanced) has compression on, so leaving the flag out when the
// device says false silently keeps it on.
func TestMapProcessingSettings_CompressDynamicRange(t *testing.T) {
	proc := NewProcessorService()

	on := proc.MapProcessingSettings(&photoframe.ProcessingSettings{CompressDynamicRange: true}, nil, ColorPalette)
	if _, ok := on["compress-dynamic-range"]; !ok {
		t.Errorf("true: expected --compress-dynamic-range, got %v", on)
	}
	if _, ok := on["no-compress-dynamic-range"]; ok {
		t.Errorf("true: unexpected --no-compress-dynamic-range in %v", on)
	}

	off := proc.MapProcessingSettings(&photoframe.ProcessingSettings{CompressDynamicRange: false}, nil, ColorPalette)
	if _, ok := off["no-compress-dynamic-range"]; !ok {
		t.Errorf("false: expected --no-compress-dynamic-range, got %v", off)
	}
	if _, ok := off["compress-dynamic-range"]; ok {
		t.Errorf("false: unexpected --compress-dynamic-range in %v", off)
	}

	// No settings at all: neither flag, the CLI keeps its defaults.
	none := proc.MapProcessingSettings(nil, nil, ColorPalette)
	for _, k := range []string{"compress-dynamic-range", "no-compress-dynamic-range"} {
		if _, ok := none[k]; ok {
			t.Errorf("nil settings: unexpected --%s in %v", k, none)
		}
	}
}

// A panel that dithers on-device must get tone-mapped full color: no
// palette (color or gray), no dithering, no compression to a measured
// palette -- even when the stored settings and palette ask for them --
// while tone options survive.
func TestMapProcessingSettings_OnDevice(t *testing.T) {
	proc := NewProcessorService()
	y := 0.02
	palette := &photoframe.Palette{
		White:  photoframe.PaletteColor{R: 200, G: 200, B: 190},
		BlackY: &y, WhiteY: &y, Gamma: &y,
	}
	settings := &photoframe.ProcessingSettings{
		Exposure:             1.2,
		Saturation:           1.1,
		Contrast:             1.3,
		ToneMode:             "contrast",
		ColorMethod:          "lab",
		DitherAlgorithm:      "stucki",
		CompressDynamicRange: true,
	}
	quantizing := []string{
		"palette", "palette-preset", "gray-black-y", "gray-white-y", "gray-gamma",
		"dither-algorithm", "color-method", "compress-dynamic-range",
	}

	for name, s := range map[string]*photoframe.ProcessingSettings{"settings": settings, "nil settings": nil} {
		opts := proc.MapProcessingSettings(s, palette, ColorOnDevice)
		for _, k := range quantizing {
			if _, ok := opts[k]; ok {
				t.Errorf("%s: unexpected --%s in %v", name, k, opts)
			}
		}
		for _, k := range []string{"skip-dithering", "no-compress-dynamic-range"} {
			if v, ok := opts[k]; !ok || v != "" {
				t.Errorf("%s: expected boolean --%s, got %v", name, k, opts)
			}
		}
	}

	opts := proc.MapProcessingSettings(settings, palette, ColorOnDevice)
	for k, want := range map[string]string{"exposure": "1.2", "saturation": "1.1", "contrast": "1.3", "tone-mode": "contrast"} {
		if opts[k] != want {
			t.Errorf("--%s = %q, want %q", k, opts[k], want)
		}
	}
}

// "rgb" wins over a grayscale board: the panel dithers on its own.
func TestColorModelFor(t *testing.T) {
	for _, tc := range []struct {
		device model.Device
		want   ColorModel
	}{
		{model.Device{}, ColorPalette},
		{model.Device{DisplayType: "spectra6"}, ColorPalette},
		{model.Device{DisplayType: "gc16"}, ColorGrayscale},
		{model.Device{BoardName: "seeedstudio_reterminal_e1003"}, ColorGrayscale},
		{model.Device{DisplayType: model.DisplayTypeRGB}, ColorOnDevice},
		{model.Device{DisplayType: model.DisplayTypeRGB, BoardName: "seeedstudio_reterminal_e1003"}, ColorOnDevice},
	} {
		if got := ColorModelFor(&tc.device); got != tc.want {
			t.Errorf("%+v: got %v, want %v", tc.device, got, tc.want)
		}
	}
	if (&model.Device{DisplayType: model.DisplayTypeRGB, BoardName: "seeedstudio_reterminal_e1003"}).IsGrayscale() {
		t.Error("an rgb device must not count as grayscale")
	}
}
