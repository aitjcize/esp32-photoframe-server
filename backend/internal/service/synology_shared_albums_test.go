package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// fakeNAS routes entry.cgi by the `api` parameter so a test can answer the
// owned-album, shared-album and item listings independently, and inspect what
// the client actually sent.
type fakeNAS struct {
	*httptest.Server

	mu sync.Mutex
	// ownedJSON/sharedJSON are the raw bodies for SYNO.Foto.Browse.Album and
	// SYNO.Foto.Sharing.Misc. itemsJSON answers SYNO.Foto.Browse.Item.
	ownedJSON, sharedJSON, itemsJSON string
	// itemQueries records the query string of every item listing.
	itemQueries []string
	sharedCalls int
}

func newFakeNAS() *fakeNAS {
	f := &fakeNAS{
		ownedJSON:  `{"success":true,"data":{"list":[]}}`,
		sharedJSON: `{"success":true,"data":{"list":[]}}`,
		itemsJSON:  `{"success":true,"data":{"list":[]}}`,
	}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "auth.cgi") {
			fmt.Fprint(w, `{"success":true,"data":{"sid":"sid-1","synotoken":"tok-1"}}`)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Query().Get("api") {
		case "SYNO.Foto.Browse.Album":
			fmt.Fprint(w, f.ownedJSON)
		case "SYNO.Foto.Sharing.Misc":
			f.sharedCalls++
			// Like DSM 7.4: only the _album method exists; others are 103.
			if r.URL.Query().Get("method") != "list_shared_with_me_album" {
				fmt.Fprint(w, `{"success":false,"error":{"code":103}}`)
				return
			}
			fmt.Fprint(w, f.sharedJSON)
		case "SYNO.Foto.Browse.Item":
			f.itemQueries = append(f.itemQueries, r.URL.RawQuery)
			fmt.Fprint(w, f.itemsJSON)
		default:
			fmt.Fprint(w, `{"success":true}`)
		}
	}))
	return f
}

func (f *fakeNAS) lastItemQuery() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.itemQueries) == 0 {
		return ""
	}
	return f.itemQueries[len(f.itemQueries)-1]
}

// newSharedAlbumService builds a Synology service over an isolated DB that has
// the album tables (unlike the settings-only shared test DB).
func newSharedAlbumService(t *testing.T, url string) (*SynologyService, *gorm.DB) {
	t.Helper()
	db := setupAlbumDB(t)
	require.NoError(t, db.AutoMigrate(&model.Setting{}))
	settings := NewSettingsService(db)
	require.NoError(t, settings.Set("synology_url", url))
	require.NoError(t, settings.Set("synology_account", "user"))
	require.NoError(t, settings.Set("synology_password", "pass"))
	return NewSynologyService(db, settings), db
}

// Issue #52: albums shared with the authenticated user live behind
// SYNO.Foto.Sharing.Misc, so listing only SYNO.Foto.Browse.Album hid every
// album the account doesn't own.
func TestSynologyListAlbumsIncludesSharedWithMe(t *testing.T) {
	nas := newFakeNAS()
	defer nas.Close()
	nas.ownedJSON = `{"success":true,"data":{"list":[{"id":1,"name":"Mine","type":"album"}]}}`
	nas.sharedJSON = `{"success":true,"data":{"list":[{"id":7,"name":"Theirs","type":"album","passphrase":"pass7"}]}}`

	svc, _ := newSharedAlbumService(t, nas.URL)
	albums, err := svc.ListAlbums()
	require.NoError(t, err)

	require.Len(t, albums, 2)
	assert.Equal(t, 1, albums[0].ID)
	assert.False(t, albums[0].SharedWithMe)
	assert.Empty(t, albums[0].Passphrase)

	assert.Equal(t, 7, albums[1].ID)
	assert.Equal(t, "Theirs", albums[1].Name)
	assert.True(t, albums[1].SharedWithMe, "a shared album must be flagged for the picker")
	assert.Equal(t, "pass7", albums[1].Passphrase)
}

// An album the user owns that is also shared back to them must appear once.
func TestSynologyListAlbumsDedupesSharedDuplicate(t *testing.T) {
	nas := newFakeNAS()
	defer nas.Close()
	nas.ownedJSON = `{"success":true,"data":{"list":[{"id":1,"name":"Mine","type":"album"}]}}`
	nas.sharedJSON = `{"success":true,"data":{"list":[{"id":1,"name":"Mine","type":"album","passphrase":"x"}]}}`

	svc, _ := newSharedAlbumService(t, nas.URL)
	albums, err := svc.ListAlbums()
	require.NoError(t, err)

	require.Len(t, albums, 1)
	assert.Equal(t, 1, albums[0].ID)
	assert.False(t, albums[0].SharedWithMe)
}

// A DSM that doesn't serve the sharing API (or an error reaching it) must not
// cost the user their owned albums.
func TestSynologyListAlbumsSurvivesSharingApiFailure(t *testing.T) {
	nas := newFakeNAS()
	defer nas.Close()
	nas.ownedJSON = `{"success":true,"data":{"list":[{"id":1,"name":"Mine","type":"album"}]}}`
	nas.sharedJSON = `{"success":false,"error":{"code":103}}`

	svc, _ := newSharedAlbumService(t, nas.URL)
	albums, err := svc.ListAlbums()
	require.NoError(t, err)

	require.Len(t, albums, 1)
	assert.Equal(t, 1, albums[0].ID)
	assert.Equal(t, 1, nas.sharedCalls)
}

// Selecting a shared album must persist its passphrase, and fetching its
// assets must present that passphrase — without it DSM refuses the browse.
func TestSynologySharedAlbumSyncUsesPassphrase(t *testing.T) {
	nas := newFakeNAS()
	defer nas.Close()
	nas.ownedJSON = `{"success":true,"data":{"list":[]}}`
	nas.sharedJSON = `{"success":true,"data":{"list":[{"id":7,"name":"Theirs","type":"album","passphrase":"pass7"}]}}`
	nas.itemsJSON = `{"success":true,"data":{"list":[{"id":900,"filename":"a.jpg","type":"photo",
		"additional":{"thumbnail":{"xl":"key-xl"},"resolution":{"width":800,"height":600}}}]}}`

	svc, db := newSharedAlbumService(t, nas.URL)
	require.NoError(t, svc.SetSyncAlbums([]string{"7"}))

	var stored model.Album
	require.NoError(t, db.Where("source = ? AND external_id = ?",
		model.SourceSynologyPhotos, "7").First(&stored).Error)
	assert.Equal(t, "pass7", stored.SharePassphrase)
	assert.True(t, stored.SyncEnabled)

	assets, err := svc.FetchAlbumAssets(stored)
	require.NoError(t, err)
	require.Len(t, assets, 1)
	assert.Equal(t, "900", assets[0].ExternalID)

	// DSM rejects album_id alongside a passphrase (error 120).
	q := nas.lastItemQuery()
	assert.NotContains(t, q, "album_id")
	assert.Contains(t, q, "passphrase=pass7")
}

// Owned albums must not gain a passphrase parameter, even when shared out by
// link: DSM then lists the link's passphrase with the owned album, and a
// browse carrying album_id and passphrase together fails with error 120.
func TestSynologyOwnedAlbumSyncSendsNoPassphrase(t *testing.T) {
	nas := newFakeNAS()
	defer nas.Close()
	nas.ownedJSON = `{"success":true,"data":{"list":[{"id":3,"name":"Mine","type":"album","shared":true,"passphrase":"link3"}]}}`
	nas.itemsJSON = `{"success":true,"data":{"list":[]}}`

	svc, db := newSharedAlbumService(t, nas.URL)
	require.NoError(t, svc.SetSyncAlbums([]string{"3"}))

	var stored model.Album
	require.NoError(t, db.Where("source = ? AND external_id = ?",
		model.SourceSynologyPhotos, "3").First(&stored).Error)
	assert.Empty(t, stored.SharePassphrase)

	_, err := svc.FetchAlbumAssets(stored)
	require.NoError(t, err)

	q := nas.lastItemQuery()
	assert.Contains(t, q, "album_id=3")
	assert.NotContains(t, q, "passphrase")
}

// The thumbnail proxy resolves an image's album through membership, so it must
// pick up the shared album's passphrase too.
func TestSynologyAlbumRefForImageCarriesPassphrase(t *testing.T) {
	nas := newFakeNAS()
	defer nas.Close()
	svc, db := newSharedAlbumService(t, nas.URL)

	album := model.Album{
		Source: model.SourceSynologyPhotos, ExternalID: "7", Kind: model.AlbumKindReal,
		Name: "Theirs", SyncEnabled: true, SharePassphrase: "pass7",
	}
	require.NoError(t, db.Create(&album).Error)
	img := model.Image{ExternalID: "900", Source: model.SourceSynologyPhotos, Status: "pending"}
	require.NoError(t, db.Create(&img).Error)
	require.NoError(t, db.Create(&model.ImageAlbumMembership{ImageID: img.ID, AlbumID: album.ID}).Error)

	ref := svc.albumRefForImage(img.ID)
	assert.Equal(t, 7, ref.ID)
	assert.Equal(t, "pass7", ref.Passphrase)
}

// recordingAlbumSource is a Synology-flavoured AlbumSource that reports what
// passphrase the engine handed it, and can advertise a rotated one.
type recordingAlbumSource struct {
	remote []RemoteAlbum
	// sawPassphrase is the SharePassphrase on the album the engine fetched.
	sawPassphrase string
}

func (f *recordingAlbumSource) Source() string { return model.SourceSynologyPhotos }
func (f *recordingAlbumSource) ListRemoteAlbums() ([]RemoteAlbum, error) {
	return f.remote, nil
}
func (f *recordingAlbumSource) FetchAlbumAssets(a model.Album) ([]RemoteAsset, error) {
	f.sawPassphrase = a.SharePassphrase
	return []RemoteAsset{{ExternalID: "900", Width: 800, Height: 600}}, nil
}

// DSM can rotate a share passphrase. The stored one must be refreshed before
// the album is fetched, or a shared album would fail every sync from then on
// with no way back short of re-picking it.
func TestSyncAlbumSourceRefreshesRotatedPassphrase(t *testing.T) {
	db := setupAlbumDB(t)
	album := model.Album{
		Source: model.SourceSynologyPhotos, ExternalID: "7", Kind: model.AlbumKindReal,
		Name: "Theirs", SyncEnabled: true, SharePassphrase: "old",
	}
	require.NoError(t, db.Create(&album).Error)

	src := &recordingAlbumSource{remote: []RemoteAlbum{
		{ExternalID: "7", Name: "Theirs", Passphrase: "new"},
	}}
	_, err := SyncAlbumSource(db, src)
	require.NoError(t, err)

	assert.Equal(t, "new", src.sawPassphrase, "the fetch must use the rotated passphrase")

	var stored model.Album
	require.NoError(t, db.First(&stored, album.ID).Error)
	assert.Equal(t, "new", stored.SharePassphrase)
}

// Saving a selection while the NAS is unreachable resolves no passphrases;
// that must leave the stored ones alone rather than blanking them.
func TestSetSyncAlbumsKeepsStoredPassphraseWhenBlank(t *testing.T) {
	db := setupAlbumDB(t)
	album := model.Album{
		Source: model.SourceSynologyPhotos, ExternalID: "7", Kind: model.AlbumKindReal,
		Name: "Theirs", SyncEnabled: false, SharePassphrase: "keep",
	}
	require.NoError(t, db.Create(&album).Error)

	require.NoError(t, SetSyncAlbums(db, model.SourceSynologyPhotos,
		[]RemoteAlbum{{ExternalID: "7", Name: "Theirs"}}))

	var stored model.Album
	require.NoError(t, db.First(&stored, album.ID).Error)
	assert.Equal(t, "keep", stored.SharePassphrase)
	assert.True(t, stored.SyncEnabled)
}
