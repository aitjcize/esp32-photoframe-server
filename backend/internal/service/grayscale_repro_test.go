package service

import (
	"encoding/json"
	"image"
	"image/color"
	"os/exec"
	"testing"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/photoframe"
)

func haveCLI(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("epaper-image-convert"); err != nil {
		t.Skip("epaper-image-convert not installed; skipping CLI repro")
	}
}

func grayImg() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(80), B: uint8(200), A: 255})
		}
	}
	return img
}

// Auto-rotate (/image) path for a grayscale device with NO stored settings and
// the firmware-reported 16-level gray palette.
func TestGrayscale_AutoRotate_FirmwarePalette(t *testing.T) {
	haveCLI(t)
	proc := NewProcessorService()

	// Exactly what the firmware reports for a GC16 panel.
	const fwPalette = `{"grays":[[0,0,0],[17,17,17],[34,34,34],[51,51,51],[68,68,68],[85,85,85],[102,102,102],[119,119,119],[136,136,136],[153,153,153],[170,170,170],[187,187,187],[204,204,204],[221,221,221],[238,238,238],[255,255,255]]}`
	var pal photoframe.Palette
	if err := json.Unmarshal([]byte(fwPalette), &pal); err != nil {
		t.Fatalf("unmarshal fw palette: %v", err)
	}

	opts := map[string]string{"dimension": "64x48", "format": "png"}
	for k, v := range proc.MapProcessingSettings(nil, &pal, true) {
		opts[k] = v
	}
	t.Logf("opts: %+v", opts)

	if _, _, err := proc.ProcessImage(grayImg(), opts); err != nil {
		t.Fatalf("ProcessImage failed (this is the 500): %v", err)
	}
}

// Grayscale device with no palette at all (empty column / header).
func TestGrayscale_AutoRotate_NoPalette(t *testing.T) {
	haveCLI(t)
	proc := NewProcessorService()
	opts := map[string]string{"dimension": "64x48", "format": "png"}
	for k, v := range proc.MapProcessingSettings(nil, nil, true) {
		opts[k] = v
	}
	t.Logf("opts: %+v", opts)
	if _, _, err := proc.ProcessImage(grayImg(), opts); err != nil {
		t.Fatalf("ProcessImage failed (this is the 500): %v", err)
	}
}

// Grayscale device whose X-Color-Palette / stored palette is a COLOR palette
// (named colors, grays empty) — e.g. a panel that was color before the
// display_type changed, or a stale palette row.
func TestGrayscale_AutoRotate_ColorPalette(t *testing.T) {
	haveCLI(t)
	proc := NewProcessorService()
	const colorPalette = `{"black":{"r":2,"g":2,"b":2},"white":{"r":190,"g":200,"b":200},"yellow":{"r":205,"g":202,"b":0},"red":{"r":135,"g":19,"b":0},"blue":{"r":5,"g":64,"b":158},"green":{"r":39,"g":102,"b":60}}`
	var pal photoframe.Palette
	if err := json.Unmarshal([]byte(colorPalette), &pal); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	opts := map[string]string{"dimension": "64x48", "format": "png"}
	for k, v := range proc.MapProcessingSettings(nil, &pal, true) {
		opts[k] = v
	}
	t.Logf("opts: %+v", opts)
	if _, _, err := proc.ProcessImage(grayImg(), opts); err != nil {
		t.Fatalf("ProcessImage failed (this is the 500): %v", err)
	}
}

// The emitted grayscale palette must carry black/white aliases in BOTH
// theoretical and perceived sub-palettes. Older epaper-image-convert versions
// (< 0.1.16) read palette.perceived.black.r directly and crash (HTTP 500) on a
// bare-grays palette; the aliases keep the palette self-contained.
func TestGrayscale_PaletteHasBlackWhiteAliases(t *testing.T) {
	proc := NewProcessorService()
	opts := proc.MapProcessingSettings(nil, nil, true)
	raw, ok := opts["palette"]
	if !ok {
		t.Fatal("expected a palette opt for a grayscale device")
	}
	var pal struct {
		Theoretical map[string]json.RawMessage `json:"theoretical"`
		Perceived   map[string]json.RawMessage `json:"perceived"`
	}
	if err := json.Unmarshal([]byte(raw), &pal); err != nil {
		t.Fatalf("palette JSON: %v", err)
	}
	for name, sub := range map[string]map[string]json.RawMessage{
		"theoretical": pal.Theoretical,
		"perceived":   pal.Perceived,
	} {
		for _, key := range []string{"grays", "black", "white"} {
			if _, ok := sub[key]; !ok {
				t.Errorf("%s sub-palette missing %q", name, key)
			}
		}
	}
}

// Grayscale device with a full processing-settings preset (the grayscale preset
// the webapp applies) plus the firmware palette.
func TestGrayscale_AutoRotate_WithSettings(t *testing.T) {
	haveCLI(t)
	proc := NewProcessorService()
	settings := &photoframe.ProcessingSettings{
		Exposure: 1, Saturation: 1, Contrast: 1, ToneMode: "contrast",
		ColorMethod: "ordered", DitherAlgorithm: "floyd-steinberg", CompressDynamicRange: true,
	}
	const fwPalette = `{"grays":[[0,0,0],[17,17,17],[34,34,34],[51,51,51],[68,68,68],[85,85,85],[102,102,102],[119,119,119],[136,136,136],[153,153,153],[170,170,170],[187,187,187],[204,204,204],[221,221,221],[238,238,238],[255,255,255]]}`
	var pal photoframe.Palette
	_ = json.Unmarshal([]byte(fwPalette), &pal)
	opts := map[string]string{"dimension": "64x48", "format": "png"}
	for k, v := range proc.MapProcessingSettings(settings, &pal, true) {
		opts[k] = v
	}
	t.Logf("opts: %+v", opts)
	if _, _, err := proc.ProcessImage(grayImg(), opts); err != nil {
		t.Fatalf("ProcessImage failed (this is the 500): %v", err)
	}
}
