package imageops

import (
	"image"
	"image/draw"
	"math"
)

// ResizeToFill resizes the source image to fill the target dimensions, cropping as necessary.
func ResizeToFill(src image.Image, targetW, targetH int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	DrawCover(dst, dst.Bounds(), src)
	return dst
}

// SmartResizeToFill behaves like ResizeToFill but automatically swaps target dimensions
// if the image orientation differs from the target orientation (Portrait/Landscape).
func SmartResizeToFill(src image.Image, targetW, targetH int) image.Image {
	// srcBounds := src.Bounds()
	// srcW, srcH := srcBounds.Dx(), srcBounds.Dy()
	// isSrcPortrait := srcH > srcW
	// isTargetPortrait := targetH > targetW

	// User requested that we respect the target orientation (device orientation)
	// even if the source is different. This usually means cropping the center
	// to fill the target frame.

	// if isSrcPortrait != isTargetPortrait {
	// 	// Orientation mismatch: swap target dimensions
	// 	targetW, targetH = targetH, targetW
	// }

	return ResizeToFill(src, targetW, targetH)
}

// DrawCover draws the source image onto the destination image, scaling and cropping to cover the destination rectangle.
// It uses a simple nearest-neighbor scaling for performance.
func DrawCover(dst draw.Image, r image.Rectangle, src image.Image) {
	// Calculate scaling to cover 'r'
	srcBounds := src.Bounds()
	srcW, srcH := srcBounds.Dx(), srcBounds.Dy()
	dstW, dstH := r.Dx(), r.Dy()

	// Calculate crop rect
	var srcCrop image.Rectangle
	if float64(srcW)/float64(srcH) > float64(dstW)/float64(dstH) {
		// Source is wider than target: Crop width
		matchW := int(float64(srcH) * float64(dstW) / float64(dstH))
		midX := srcW / 2
		srcCrop = image.Rect(midX-matchW/2, 0, midX+matchW/2, srcH)
	} else {
		// Source is taller: Crop height
		matchH := int(float64(srcW) * float64(dstH) / float64(dstW))
		midY := srcH / 2
		srcCrop = image.Rect(0, midY-matchH/2, srcW, midY+matchH/2)
	}

	// Implementation of simple Nearest Neighbor scaler:
	for y := 0; y < dstH; y++ {
		for x := 0; x < dstW; x++ {
			// Percentages in Destination
			pX := float64(x) / float64(dstW)
			pY := float64(y) / float64(dstH)

			// Source coords in original image via Crop
			sX := srcCrop.Min.X + int(pX*float64(srcCrop.Dx()))
			sY := srcCrop.Min.Y + int(pY*float64(srcCrop.Dy()))

			// Bounds check safety
			sX = int(math.Min(math.Max(0, float64(sX)), float64(srcW-1)))
			sY = int(math.Min(math.Max(0, float64(sY)), float64(srcH-1)))

			dst.Set(r.Min.X+x, r.Min.Y+y, src.At(srcBounds.Min.X+sX, srcBounds.Min.Y+sY))
		}
	}
}
