package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// pagingNAS serves owned albums, shared albums and album items out of
// fixed-size collections, honouring offset/limit the way DSM does, and counts
// the list requests per api.
type pagingNAS struct {
	*httptest.Server

	mu    sync.Mutex
	calls map[string]int
}

// newPagingNAS serves owned albums with ids 1..owned, shared albums with ids
// 100000+1..100000+shared, and photos with ids 1..photos in every album.
func newPagingNAS(owned, shared, photos int) *pagingNAS {
	f := &pagingNAS{calls: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "auth.cgi") {
			fmt.Fprint(w, `{"success":true,"data":{"sid":"sid-1","synotoken":"tok-1"}}`)
			return
		}
		q := r.URL.Query()
		api := q.Get("api")
		f.mu.Lock()
		f.calls[api]++
		f.mu.Unlock()
		offset, _ := strconv.Atoi(q.Get("offset"))
		limit, _ := strconv.Atoi(q.Get("limit"))
		var total, idBase int
		var item func(id int) map[string]any
		switch api {
		case "SYNO.Foto.Browse.Album":
			total, item = owned, func(id int) map[string]any {
				return map[string]any{"id": id, "name": fmt.Sprintf("Album %d", id), "type": "album"}
			}
		case "SYNO.Foto.Sharing.Misc":
			total, idBase = shared, 100000
			item = func(id int) map[string]any {
				return map[string]any{"id": id, "name": fmt.Sprintf("Shared %d", id), "type": "album", "passphrase": "p"}
			}
		case "SYNO.Foto.Browse.Item":
			total, item = photos, func(id int) map[string]any {
				return map[string]any{"id": id, "filename": fmt.Sprintf("%d.jpg", id), "type": "photo",
					"additional": map[string]any{"resolution": map[string]int{"width": 800, "height": 600}}}
			}
		default:
			fmt.Fprint(w, `{"success":true}`)
			return
		}
		list := []map[string]any{}
		for i := offset; i < offset+limit && i < total; i++ {
			list = append(list, item(idBase+i+1))
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"list": list}})
	}))
	return f
}

func (f *pagingNAS) callCount(api string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[api]
}

// Issue #56: both album listings stopped at the first 100.
func TestSynologyListAlbumsPagesPastFirstHundred(t *testing.T) {
	nas := newPagingNAS(250, 130, 0)
	defer nas.Close()

	svc, _ := newSharedAlbumService(t, nas.URL)
	albums, err := svc.ListAlbums()
	require.NoError(t, err)

	require.Len(t, albums, 380)
	assert.Equal(t, 250, albums[249].ID)
	assert.False(t, albums[249].SharedWithMe)
	assert.Equal(t, 100130, albums[379].ID)
	assert.True(t, albums[379].SharedWithMe)
	// 100+100+50 owned, 100+30 shared: each listing ends on its short page.
	assert.Equal(t, 3, nas.callCount("SYNO.Foto.Browse.Album"))
	assert.Equal(t, 2, nas.callCount("SYNO.Foto.Sharing.Misc"))
}

// Issue #56: album assets were silently capped at 5,000.
func TestSynologyFetchAlbumAssetsPagesPastFiveThousand(t *testing.T) {
	nas := newPagingNAS(0, 0, 5200)
	defer nas.Close()

	svc, _ := newSharedAlbumService(t, nas.URL)
	require.NoError(t, mustLogin(svc, "", false))
	assets, err := svc.FetchAlbumAssets(model.Album{
		Source: model.SourceSynologyPhotos, ExternalID: "3", Kind: model.AlbumKindReal,
	})
	require.NoError(t, err)

	require.Len(t, assets, 5200)
	assert.Equal(t, "5200", assets[5199].ExternalID)
	assert.Equal(t, 11, nas.callCount("SYNO.Foto.Browse.Item"))
}

// A server that never returns a short page (e.g. one that ignores offset) must
// not loop forever: paging stops at the bound and keeps what it read.
func TestPageSynologyStopsAtBound(t *testing.T) {
	calls := 0
	got, err := pageSynology("test", 10, 5, func(offset, limit int) ([]int, error) {
		calls++
		return make([]int, limit), nil
	})
	require.NoError(t, err)
	assert.Equal(t, 5, calls)
	assert.Len(t, got, 50)
}

// An exact multiple of the page size ends on the empty page that follows, and
// an error on any page fails the whole listing.
func TestPageSynologyEmptyLastPageAndError(t *testing.T) {
	got, err := pageSynology("test", 10, 5, func(offset, limit int) ([]int, error) {
		if offset >= 20 {
			return nil, nil
		}
		return make([]int, limit), nil
	})
	require.NoError(t, err)
	assert.Len(t, got, 20)

	_, err = pageSynology("test", 10, 5, func(offset, limit int) ([]int, error) {
		if offset >= 10 {
			return nil, errors.New("boom")
		}
		return make([]int, limit), nil
	})
	assert.EqualError(t, err, "boom")
}
