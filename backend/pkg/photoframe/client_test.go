package photoframe

import (
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   int
	}{
		{"2.6.1", "2.6.1", 0},
		{"2.6.2", "2.6.1", 1},
		{"2.6.0", "2.6.1", -1},
		{"2.7.0", "2.6.1", 1},
		{"3.0.0", "2.6.1", 1},
		{"1.0.0", "2.6.1", -1},
		// v prefix
		{"v2.6.2", "2.6.1", 1},
		{"2.6.2", "v2.6.1", 1},
		{"v2.6.1", "v2.6.1", 0},
		// dev versions
		{"dev-abc123", "2.6.1", -1},
		{"2.6.1", "dev-abc123", 1},
		// short versions
		{"2.7", "2.6.1", 1},
		{"2", "2.6.1", -1},
	}

	for _, tt := range tests {
		got := compareVersions(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
		}
	}
}

func TestSupportsEPDGZ(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		// Must be strictly greater than 2.6.1
		{"2.6.1", false},
		{"2.6.0", false},
		{"2.5.0", false},
		{"1.0.0", false},
		{"2.6.2", true},
		{"2.7.0", true},
		{"3.0.0", true},
		// v prefix
		{"v2.6.2", true},
		{"v2.6.1", false},
		// dev builds run the latest code -> treated as supporting epdgz
		{"dev-abc123", true},
		// empty string
		{"", false},
	}

	for _, tt := range tests {
		got := SupportsEPDGZ(tt.version)
		if got != tt.want {
			t.Errorf("SupportsEPDGZ(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}
