package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// convertToJPEG runs image bytes of any format ImageMagick reads (HEIC, RAW,
// WebP, PNG, JPEG, ...) through `magick -auto-orient`, returning a JPEG with
// the EXIF orientation applied to the pixels. Go's decoders ignore EXIF
// orientation and know neither HEIC nor WebP, so downloaded originals pass
// through here before they are decoded.
func convertToJPEG(data []byte) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "photo-convert-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input")
	outputPath := filepath.Join(tmpDir, "output.jpg")
	if err := os.WriteFile(inputPath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}
	cmd := exec.Command("magick", inputPath, "-auto-orient", "-quality", "95", outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ImageMagick conversion failed: %v (output: %s)", err, string(output))
	}
	return os.ReadFile(outputPath)
}
