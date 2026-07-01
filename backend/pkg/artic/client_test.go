package artic

import "testing"

// TestSearchLive hits the real ARTIC API; skipped under `go test -short` (CI).
func TestSearchLive(t *testing.T) {
	if testing.Short() {
		t.Skip("live network test")
	}
	arts, err := New().Search("gelatin silver print", 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(arts) == 0 {
		t.Fatal("no results")
	}
	for _, a := range arts {
		if a.ImageID == "" {
			t.Errorf("missing image_id: %+v", a)
		}
		if a.ImageURL() == "" {
			t.Errorf("empty image url: %+v", a)
		}
		if a.Thumbnail.Width == 0 || a.Thumbnail.Height == 0 {
			t.Errorf("missing thumbnail dims (needed for orientation): %+v", a)
		}
	}
}
