package immich

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestClient returns a Client whose requests are routed to the given test
// server, bypassing the mdns transport used by NewClient.
func newTestClient(baseURL string) *Client {
	return &Client{
		BaseURL:    baseURL,
		APIKey:     "test-key",
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// memoriesServer returns a test server that serves two "on this day" lanes
// (2022 with one asset, 2024 with two) and records the query params it saw.
func memoriesServer(t *testing.T, gotFor, gotType *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/memories" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		*gotFor = r.URL.Query().Get("for")
		*gotType = r.URL.Query().Get("type")
		lane2022 := MemoryLane{ID: "lane-2022", Assets: []Asset{{ID: "a1", Type: "IMAGE"}}}
		lane2022.Data.Year = 2022
		lane2024 := MemoryLane{ID: "lane-2024", Assets: []Asset{{ID: "a2", Type: "IMAGE"}, {ID: "a3", Type: "IMAGE"}}}
		lane2024.Data.Year = 2024
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]MemoryLane{lane2022, lane2024})
	}))
}

// GetMemoryAssets must scope the request to today via the `for` query
// parameter (and type=on_this_day); without it Immich returns every memory
// lane instead of just "on this day" — see issue #32. In the default (all)
// mode every lane is flattened into one pool.
func TestGetMemoryAssets_ScopesToToday(t *testing.T) {
	var gotFor, gotType string
	srv := memoriesServer(t, &gotFor, &gotType)
	defer srv.Close()

	assets, err := newTestClient(srv.URL).GetMemoryAssets(false)
	if err != nil {
		t.Fatalf("GetMemoryAssets: %v", err)
	}

	if gotType != "on_this_day" {
		t.Errorf("type query param = %q, want %q", gotType, "on_this_day")
	}
	if gotFor == "" {
		t.Fatal("for query param was not sent")
	}
	// The `for` value must be today's date so memories are scoped correctly.
	parsed, err := time.Parse("2006-01-02T15:04:05.000Z", gotFor)
	if err != nil {
		t.Fatalf("for query param %q not in expected format: %v", gotFor, err)
	}
	today := time.Now().UTC()
	if parsed.Year() != today.Year() || parsed.YearDay() != today.YearDay() {
		t.Errorf("for query param date = %v, want today %v", parsed, today)
	}

	// Lanes must be flattened into a single asset slice.
	if len(assets) != 3 {
		t.Errorf("got %d assets, want 3 (flattened across lanes)", len(assets))
	}
}

// In latest-year mode only the most recent year's lane is returned.
func TestGetMemoryAssets_LatestYearOnly(t *testing.T) {
	var gotFor, gotType string
	srv := memoriesServer(t, &gotFor, &gotType)
	defer srv.Close()

	assets, err := newTestClient(srv.URL).GetMemoryAssets(true)
	if err != nil {
		t.Fatalf("GetMemoryAssets: %v", err)
	}

	// 2024 is the most recent lane and has assets a2, a3.
	if len(assets) != 2 {
		t.Fatalf("got %d assets, want 2 (most recent year's lane only)", len(assets))
	}
	if assets[0].ID != "a2" || assets[1].ID != "a3" {
		t.Errorf("got assets %v, want [a2 a3] from the 2024 lane", assets)
	}
}
