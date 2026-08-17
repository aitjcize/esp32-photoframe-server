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
		BaseURL:        baseURL,
		APIKey:         "test-key",
		httpClient:     &http.Client{Timeout: 5 * time.Second},
		downloadClient: &http.Client{Timeout: 5 * time.Second},
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
// Asset byte fetches must request the edited rendition (edited=true), so
// crops/rotations made in the Immich editor reach the frame instead of the
// untouched original — see issue #46.
func TestDownloadOriginal_RequestsEditedRendition(t *testing.T) {
	var gotEdited []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/assets/a1/original" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		gotEdited = append(gotEdited, r.URL.Query().Get("edited"))
		_, _ = w.Write([]byte("image-bytes"))
	}))
	defer srv.Close()

	data, err := newTestClient(srv.URL).DownloadOriginal("a1")
	if err != nil {
		t.Fatalf("DownloadOriginal: %v", err)
	}
	if string(data) != "image-bytes" {
		t.Errorf("got body %q, want image-bytes", data)
	}
	if len(gotEdited) != 1 || gotEdited[0] != "true" {
		t.Errorf("edited query params = %v, want [true]", gotEdited)
	}
}

// A server that rejects the edited parameter with a 400 must get a retry
// without it, keeping asset fetches working on Immich versions that predate
// the editor.
func TestGetThumbnail_EditedParamRejectedFallsBack(t *testing.T) {
	var sizes, editeds []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/assets/a1/thumbnail" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		sizes = append(sizes, r.URL.Query().Get("size"))
		editeds = append(editeds, r.URL.Query().Get("edited"))
		if r.URL.Query().Get("edited") != "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte("thumb-bytes"))
	}))
	defer srv.Close()

	data, err := newTestClient(srv.URL).GetThumbnail("a1", "preview")
	if err != nil {
		t.Fatalf("GetThumbnail: %v", err)
	}
	if string(data) != "thumb-bytes" {
		t.Errorf("got body %q, want thumb-bytes", data)
	}
	if len(editeds) != 2 || editeds[0] != "true" || editeds[1] != "" {
		t.Errorf("edited query params = %v, want [true, empty]", editeds)
	}
	if len(sizes) != 2 || sizes[0] != "preview" || sizes[1] != "preview" {
		t.Errorf("size query params = %v, want preview on both requests", sizes)
	}
}

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
	// The `for` value must be today's local date, date-only — Immich v3.0.3+
	// rejects a timestamp with a time component (issue #44).
	parsed, err := time.Parse(time.DateOnly, gotFor)
	if err != nil {
		t.Fatalf("for query param %q not in expected format: %v", gotFor, err)
	}
	today := time.Now()
	if parsed.Year() != today.Year() || parsed.YearDay() != today.YearDay() {
		t.Errorf("for query param date = %v, want today %v", parsed, today)
	}

	// Lanes must be flattened into a single asset slice.
	if len(assets) != 3 {
		t.Errorf("got %d assets, want 3 (flattened across lanes)", len(assets))
	}
}

// Immich v3.0.0–v3.0.2 validate `for` as a full ISO datetime and reject the
// date-only form with a 400; the client must retry with the legacy timestamp
// format so those versions keep working.
func TestGetMemoryAssets_LegacyDatetimeFallback(t *testing.T) {
	var forValues []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forValue := r.URL.Query().Get("for")
		forValues = append(forValues, forValue)
		if _, err := time.Parse("2006-01-02T15:04:05.000Z", forValue); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"Validation failed"}`))
			return
		}
		lane := MemoryLane{ID: "lane-2023", Assets: []Asset{{ID: "a1", Type: "IMAGE"}}}
		lane.Data.Year = 2023
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]MemoryLane{lane})
	}))
	defer srv.Close()

	assets, err := newTestClient(srv.URL).GetMemoryAssets(false)
	if err != nil {
		t.Fatalf("GetMemoryAssets: %v", err)
	}
	if len(forValues) != 2 {
		t.Fatalf("got %d requests, want 2 (date-only, then legacy retry)", len(forValues))
	}
	if _, err := time.Parse(time.DateOnly, forValues[0]); err != nil {
		t.Errorf("first request for=%q, want date-only format", forValues[0])
	}
	if len(assets) != 1 || assets[0].ID != "a1" {
		t.Errorf("got assets %v, want [a1] from the retried request", assets)
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
