package nextcloud

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newTestClient(baseURL string) *Client {
	return &Client{
		BaseURL:        baseURL,
		Username:       "alice",
		AppPassword:    "app-pass",
		httpClient:     &http.Client{Timeout: 5 * time.Second},
		downloadClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// dayPhotosHandler serves GET /api/days/{ids} from a dayid → photos map and
// records each request's ids and query.
type dayPhotosHandler struct {
	photos   map[int64][]Photo
	requests [][]int64
	queries  []string
}

func (h *dayPhotosHandler) serve(t *testing.T, w http.ResponseWriter, r *http.Request) {
	idsPart := strings.TrimPrefix(r.URL.Path, memoriesAPI+"/days/")
	var ids []int64
	var out []Photo
	for _, s := range strings.Split(idsPart, ",") {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			t.Errorf("bad day id %q", s)
			continue
		}
		ids = append(ids, id)
		out = append(out, h.photos[id]...)
	}
	h.requests = append(h.requests, ids)
	h.queries = append(h.queries, r.URL.RawQuery)
	writeJSON(w, out)
}

func TestRequestsCarryAuthAndOCSHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "alice" || pass != "app-pass" {
			t.Errorf("basic auth = %q/%q/%v", user, pass, ok)
		}
		if r.Header.Get("OCS-APIRequest") != "true" {
			t.Error("missing OCS-APIRequest header")
		}
		writeJSON(w, []Album{})
	}))
	defer srv.Close()
	if _, err := newTestClient(srv.URL).ListAlbums(); err != nil {
		t.Fatal(err)
	}
}

func TestListAlbumsDecodesClusterIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != memoriesAPI+"/clusters/albums" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{"album_id": 3, "cluster_id": "alice/Holiday", "name": "Holiday", "user": "alice", "count": 12},
			{"album_id": 9, "cluster_id": 9, "name": "Shared", "user": "bob", "count": 4}
		]`))
	}))
	defer srv.Close()
	albums, err := newTestClient(srv.URL).ListAlbums()
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 2 || albums[0].ClusterID != "alice/Holiday" || albums[1].ClusterID != "9" {
		t.Fatalf("albums = %+v", albums)
	}
}

func TestTestConnection(t *testing.T) {
	cases := []struct {
		name       string
		userStatus int
		memStatus  int
		want       error
	}{
		{"ok", 200, 200, nil},
		{"bad password", 401, 200, ErrUnauthorized},
		{"no memories", 200, 404, ErrMemoriesMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/ocs/v2.php/cloud/user":
					w.WriteHeader(tc.userStatus)
					_, _ = w.Write([]byte(`{}`))
				case memoriesAPI + "/clusters/albums":
					w.WriteHeader(tc.memStatus)
					_, _ = w.Write([]byte(`[]`))
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			}))
			defer srv.Close()
			err := newTestClient(srv.URL).TestConnection()
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestGetPhotosBatchesDaysAndKeepsFilter: an album with more days than one
// request carries is fetched in batches, each scoped to the album.
func TestGetPhotosBatchesDaysAndKeepsFilter(t *testing.T) {
	h := &dayPhotosHandler{photos: map[int64][]Photo{}}
	var days []Day
	for i := int64(1); i <= dayBatch+7; i++ {
		days = append(days, Day{DayID: 19000 + i, Count: 1})
		h.photos[19000+i] = []Photo{{FileID: i, DayID: 19000 + i, Mimetype: "image/jpeg"}}
	}
	days = append(days, Day{DayID: 1, Count: 0}) // empty days are not requested
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == memoriesAPI+"/days" {
			if got := r.URL.Query().Get("albums"); got != "alice/Holiday" {
				t.Errorf("days albums filter = %q", got)
			}
			writeJSON(w, days)
			return
		}
		h.serve(t, w, r)
	}))
	defer srv.Close()

	photos, err := newTestClient(srv.URL).GetPhotos(Filter{Album: "alice/Holiday"})
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != dayBatch+7 {
		t.Fatalf("got %d photos, want %d", len(photos), dayBatch+7)
	}
	if len(h.requests) != 2 || len(h.requests[0]) != dayBatch || len(h.requests[1]) != 7 {
		t.Fatalf("batches = %d/%v", len(h.requests), h.requests)
	}
	for _, q := range h.queries {
		if q != "albums=alice%2FHoliday" {
			t.Errorf("day query = %q", q)
		}
	}
}

func TestFavoritesFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fav") != "1" {
			t.Errorf("%s without fav=1", r.URL.Path)
		}
		if r.URL.Path == memoriesAPI+"/days" {
			writeJSON(w, []Day{{DayID: 5, Count: 1}})
			return
		}
		writeJSON(w, []Photo{{FileID: 1, DayID: 5}})
	}))
	defer srv.Close()
	photos, err := newTestClient(srv.URL).GetPhotos(Filter{Favorites: true})
	if err != nil || len(photos) != 1 {
		t.Fatalf("photos=%v err=%v", photos, err)
	}
}

func TestOnThisDayIDs(t *testing.T) {
	now := time.Date(2026, time.November, 19, 15, 0, 0, 0, time.Local)
	ids := onThisDayIDs(now, 3)
	want := []int64{DayID(2025, 11, 19), DayID(2024, 11, 19), DayID(2023, 11, 19)}
	if len(ids) != 3 || ids[0] != want[0] || ids[1] != want[1] || ids[2] != want[2] {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	// 1970-01-02 is day 1.
	if DayID(1970, 1, 2) != 1 {
		t.Fatalf("DayID(1970-01-02) = %d", DayID(1970, 1, 2))
	}
}

func TestOnThisDayIDsLeapDay(t *testing.T) {
	now := time.Date(2028, time.February, 29, 9, 0, 0, 0, time.UTC)
	ids := onThisDayIDs(now, 8)
	want := []int64{DayID(2024, 2, 29), DayID(2020, 2, 29)}
	if len(ids) != 2 || ids[0] != want[0] || ids[1] != want[1] {
		t.Fatalf("ids = %v, want %v (only leap years)", ids, want)
	}
}

func TestGetOnThisDay(t *testing.T) {
	now := time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC)
	d2025, d2024, d2020 := DayID(2025, 6, 1), DayID(2024, 6, 1), DayID(2020, 6, 1)
	h := &dayPhotosHandler{photos: map[int64][]Photo{
		// The newest year only has a video: latest mode must fall back.
		d2025: {{FileID: 1, DayID: d2025, IsVideo: true, Mimetype: "video/mp4"}},
		d2024: {{FileID: 2, DayID: d2024, Mimetype: "image/jpeg"}, {FileID: 3, DayID: d2024, Mimetype: "image/heic"}},
		d2020: {{FileID: 4, DayID: d2020, Mimetype: "image/jpeg"}},
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == memoriesAPI+"/days" {
			writeJSON(w, []Day{
				{DayID: d2025, Count: 1}, {DayID: d2024, Count: 2}, {DayID: d2020, Count: 1},
				{DayID: DayID(2024, 6, 2), Count: 9}, // not this day
			})
			return
		}
		h.serve(t, w, r)
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)

	all, err := c.GetOnThisDay(false, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("all years: got %d photos, want 4", len(all))
	}
	if len(h.requests) != 1 || len(h.requests[0]) != 3 {
		t.Fatalf("requested days = %v, want only the 3 matching days", h.requests)
	}

	latest, err := c.GetOnThisDay(true, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) != 2 || latest[0].FileID != 2 {
		t.Fatalf("latest = %+v, want the 2024 photos", latest)
	}
}

func TestIsServableImage(t *testing.T) {
	cases := []struct {
		p    Photo
		want bool
	}{
		{Photo{FileID: 1, Basename: "a.jpg", Mimetype: "image/jpeg"}, true},
		{Photo{FileID: 1, Basename: "a.heic", Mimetype: "image/heic"}, true},
		{Photo{FileID: 1, Basename: "a.mp4", Mimetype: "video/mp4", IsVideo: true}, false},
		{Photo{FileID: 1, Basename: "a.dng", Mimetype: "image/x-adobe-dng"}, false},
		{Photo{FileID: 1, Basename: "a.jpg", IsHidden: true}, false},
		{Photo{Basename: "a.jpg"}, false},
	}
	for _, tc := range cases {
		if got := IsServableImage(tc.p); got != tc.want {
			t.Errorf("IsServableImage(%+v) = %v, want %v", tc.p, got, tc.want)
		}
	}
}

func TestPhotoDecodesLooseBooleans(t *testing.T) {
	var ps []Photo
	err := json.Unmarshal([]byte(`[{"fileid":1,"isvideo":1},{"fileid":2,"isvideo":"0"},{"fileid":3,"isvideo":true}]`), &ps)
	if err != nil {
		t.Fatal(err)
	}
	if !bool(ps[0].IsVideo) || bool(ps[1].IsVideo) || !bool(ps[2].IsVideo) {
		t.Fatalf("isvideo = %v %v %v", ps[0].IsVideo, ps[1].IsVideo, ps[2].IsVideo)
	}
}

func TestTakenAt(t *testing.T) {
	if got := (Photo{DateTaken: 86400 * 3}).TakenAt(); got == nil || got.Unix() != 86400*3 {
		t.Fatalf("datetaken: %v", got)
	}
	if got := (Photo{DayID: 2}).TakenAt(); got == nil || got.Unix() != 2*86400 {
		t.Fatalf("dayid: %v", got)
	}
	if (Photo{}).TakenAt() != nil {
		t.Fatal("no date should give nil")
	}
}
