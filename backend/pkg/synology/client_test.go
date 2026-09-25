package synology

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DSM stores thumbnails under a unit_id that differs from the item id for many
// newer items. type "unit" looks the id up as a unit, so passing the item id
// there 404s; type "item" resolves the item's own thumbnail.
func TestGetPhotoRequestsByItemType(t *testing.T) {
	// Mirrors a real DSM 7.4 item: id 238228, thumbnail unit_id 236492.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		id := q.Get("id")
		switch {
		case q.Get("type") == `"item"` && id == "238228",
			q.Get("type") == `"unit"` && id == "236492":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte("jpeg"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "user", "pass", false)
	require.NoError(t, err)

	data, err := c.GetPhoto(238228, "236492_1773509412", "small", AlbumRef{ID: 344}, "")
	require.NoError(t, err)
	assert.Equal(t, "jpeg", string(data))
}

func TestItemDecodesThumbnailCacheKey(t *testing.T) {
	raw := `{"id":238228,"type":"live","additional":{"thumbnail":{"m":"ready","xl":"ready","preview":"broken","sm":"ready","cache_key":"236492_1773509412","unit_id":236492}}}`
	var it Item
	require.NoError(t, json.Unmarshal([]byte(raw), &it))
	assert.Equal(t, "236492_1773509412", it.Additional.Thumbnail.CacheKey)
	assert.Equal(t, 236492, it.Additional.Thumbnail.UnitID)
	assert.Equal(t, "ready", it.Additional.Thumbnail.XL)
	assert.Equal(t, "ready", it.Additional.Thumbnail.S)
}

// Shared albums are reached by passphrase alone: DSM answers 404 to a
// thumbnail request that also carries album_id.
func TestGetPhotoSendsPassphraseInsteadOfAlbumID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("passphrase") == `"pass7"` && !q.Has("album_id") {
			w.Write([]byte("jpeg"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "user", "pass", false)
	require.NoError(t, err)

	data, err := c.GetPhoto(900, "900_1", "small", AlbumRef{ID: 7, Passphrase: "pass7"}, "")
	require.NoError(t, err)
	assert.Equal(t, "jpeg", string(data))
}
