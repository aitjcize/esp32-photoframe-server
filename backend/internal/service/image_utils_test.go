package service

import "testing"

func TestDetermineOrientation(t *testing.T) {
	tests := []struct {
		name   string
		w, h   int
		exif   string
		expect string
	}{
		{"landscape", 200, 100, "", "landscape"},
		{"portrait", 100, 200, "", "portrait"},
		{"square is landscape", 100, 100, "", "landscape"},
		// Unknown dims (Synology omits resolution) must NOT default to landscape.
		{"zero both -> auto", 0, 0, "", "auto"},
		{"zero width -> auto", 0, 200, "", "auto"},
		{"zero height -> auto", 200, 0, "", "auto"},
		{"negative -> auto", -1, 100, "", "auto"},
		// EXIF 90/270 swaps w/h before comparison.
		{"landscape pixels + EXIF 6 -> portrait", 200, 100, "6", "portrait"},
		{"portrait pixels + EXIF 8 -> landscape", 100, 200, "8", "landscape"},
		{"landscape pixels + 'Rotate 90 CW' -> portrait", 200, 100, "Rotate 90 CW", "portrait"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := determineOrientation(tt.w, tt.h, tt.exif); got != tt.expect {
				t.Errorf("determineOrientation(%d, %d, %q) = %q, want %q",
					tt.w, tt.h, tt.exif, got, tt.expect)
			}
		})
	}
}
