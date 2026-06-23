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

// GetMemoryAssets must scope the request to today via the `for` query
// parameter (and type=on_this_day); without it Immich returns every memory
// lane instead of just "on this day" — see issue #32.
func TestGetMemoryAssets_ScopesToToday(t *testing.T) {
	var gotFor, gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/memories" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotFor = r.URL.Query().Get("for")
		gotType = r.URL.Query().Get("type")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]MemoryLane{
			{ID: "lane-1", Assets: []Asset{{ID: "a1", Type: "IMAGE"}}},
			{ID: "lane-2", Assets: []Asset{{ID: "a2", Type: "IMAGE"}, {ID: "a3", Type: "IMAGE"}}},
		})
	}))
	defer srv.Close()

	assets, err := newTestClient(srv.URL).GetMemoryAssets()
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
