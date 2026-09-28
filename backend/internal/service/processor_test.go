package service

import (
	"testing"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/photoframe"
)

// compressDynamicRange must reach the CLI in both states. The CLI's default
// preset (balanced) has compression on, so leaving the flag out when the
// device says false silently keeps it on.
func TestMapProcessingSettings_CompressDynamicRange(t *testing.T) {
	proc := NewProcessorService()

	on := proc.MapProcessingSettings(&photoframe.ProcessingSettings{CompressDynamicRange: true}, nil, false)
	if _, ok := on["compress-dynamic-range"]; !ok {
		t.Errorf("true: expected --compress-dynamic-range, got %v", on)
	}
	if _, ok := on["no-compress-dynamic-range"]; ok {
		t.Errorf("true: unexpected --no-compress-dynamic-range in %v", on)
	}

	off := proc.MapProcessingSettings(&photoframe.ProcessingSettings{CompressDynamicRange: false}, nil, false)
	if _, ok := off["no-compress-dynamic-range"]; !ok {
		t.Errorf("false: expected --no-compress-dynamic-range, got %v", off)
	}
	if _, ok := off["compress-dynamic-range"]; ok {
		t.Errorf("false: unexpected --compress-dynamic-range in %v", off)
	}

	// No settings at all: neither flag, the CLI keeps its defaults.
	none := proc.MapProcessingSettings(nil, nil, false)
	for _, k := range []string{"compress-dynamic-range", "no-compress-dynamic-range"} {
		if _, ok := none[k]; ok {
			t.Errorf("nil settings: unexpected --%s in %v", k, none)
		}
	}
}
