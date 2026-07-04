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

// TestGetAlbumAssets_V2Inline covers Immich v2, which embeds the asset array
// directly in GET /api/albums/{id}?withAssets=true. No search fallback fires.
func TestGetAlbumAssets_V2Inline(t *testing.T) {
	var searchCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/albums/alb1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(AlbumDetail{
				ID: "alb1", AlbumName: "A", AssetCount: 2,
				Assets: []Asset{{ID: "a1", Type: "IMAGE"}, {ID: "a2", Type: "IMAGE"}},
			})
		case "/api/search/metadata":
			searchCalled = true
			t.Error("search fallback should not fire when assets are inlined")
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	assets, err := newTestClient(srv.URL).GetAlbumAssets("alb1")
	if err != nil {
		t.Fatalf("GetAlbumAssets: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("got %d assets, want 2", len(assets))
	}
	if searchCalled {
		t.Error("search endpoint was called unexpectedly")
	}
}

// TestGetAlbumAssets_V3SearchFallback covers Immich v3 (>= 3.0), which returns
// assetCount but no inline asset array. GetAlbumAssets must page the album via
// POST /api/search/metadata scoped by albumIds, requesting exif.
func TestGetAlbumAssets_V3SearchFallback(t *testing.T) {
	var gotAlbumIDs []string
	var gotWithExif bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/albums/alb1":
			_ = json.NewEncoder(w).Encode(AlbumDetail{ID: "alb1", AlbumName: "A", AssetCount: 3})
		case "/api/search/metadata":
			var req SearchMetadataRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			gotAlbumIDs = req.AlbumIDs
			gotWithExif = req.WithExif
			var body searchAssetsResponse
			body.Assets.Items = []Asset{
				{ID: "a1", Type: "IMAGE"}, {ID: "a2", Type: "IMAGE"}, {ID: "a3", Type: "IMAGE"},
			}
			_ = json.NewEncoder(w).Encode(body)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	assets, err := newTestClient(srv.URL).GetAlbumAssets("alb1")
	if err != nil {
		t.Fatalf("GetAlbumAssets: %v", err)
	}
	if len(assets) != 3 {
		t.Fatalf("got %d assets, want 3 (from search fallback)", len(assets))
	}
	if len(gotAlbumIDs) != 1 || gotAlbumIDs[0] != "alb1" {
		t.Errorf("search albumIds = %v, want [alb1]", gotAlbumIDs)
	}
	if !gotWithExif {
		t.Error("search request must set withExif=true (v3 omits exif otherwise)")
	}
}

// TestGetAlbumAssets_EmptyAlbum verifies a genuinely empty album (assetCount 0,
// no inline assets) returns no assets without hitting the search endpoint.
func TestGetAlbumAssets_EmptyAlbum(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/search/metadata" {
			t.Error("search fallback should not fire for an empty album")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AlbumDetail{ID: "alb1", AlbumName: "A", AssetCount: 0})
	}))
	defer srv.Close()

	assets, err := newTestClient(srv.URL).GetAlbumAssets("alb1")
	if err != nil {
		t.Fatalf("GetAlbumAssets: %v", err)
	}
	if len(assets) != 0 {
		t.Fatalf("got %d assets, want 0", len(assets))
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
