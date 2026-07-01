package service

import "strings"

// isRotatedOrientation returns true if the EXIF orientation indicates a 90° or 270°
// rotation, meaning width and height should be swapped to get the display dimensions.
// Handles both numeric ("5"-"8") and descriptive ("Rotate 90 CW") formats.
func isRotatedOrientation(orientation string) bool {
	switch orientation {
	case "5", "6", "7", "8":
		return true
	}
	return strings.Contains(orientation, "90") || strings.Contains(orientation, "270")
}

// determineOrientation returns "portrait", "landscape", or "auto" based on image
// dimensions and optional EXIF orientation. If exifOrientation indicates a
// 90°/270° rotation, width and height are swapped before comparison. When the
// dimensions are unknown (either is <= 0) it returns "auto" rather than guessing
// -- a source that omits resolution must not be silently labeled landscape.
func determineOrientation(width, height int, exifOrientation string) string {
	if width <= 0 || height <= 0 {
		return "auto"
	}
	if isRotatedOrientation(exifOrientation) {
		width, height = height, width
	}
	if height > width {
		return "portrait"
	}
	return "landscape"
}
