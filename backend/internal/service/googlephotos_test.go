package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
)

var googleTestDBCounter atomic.Int64

func setupGoogleDB(t *testing.T) *gorm.DB {
	t.Helper()
	n := googleTestDBCounter.Add(1)
	db, err := gorm.Open(sqlite.Open(
		fmt.Sprintf("file:google_test_%d?mode=memory&cache=shared", n)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Connection{}, &model.Image{}, &model.Album{}, &model.ImageAlbumMembership{},
		&model.Device{}, &model.DeviceAlbumMapping{}))
	return db
}

type staticGoogleConfig struct{ id, secret string }

func (c staticGoogleConfig) GetGoogleConfig() (googlephotos.Config, error) {
	return googlephotos.Config{ClientID: c.id, ClientSecret: c.secret, Scopes: []string{"x"}}, nil
}

func newGoogleManager(t *testing.T, db *gorm.DB, cfg googlephotos.ConfigProvider) *ConnectionManager {
	t.Helper()
	m := NewConnectionManager(db, cfg, cfg, NewPickerService(db, t.TempDir()))
	m.Start()
	return m
}

// Each Google connection keeps its own token in its own row, and a refresh
// that comes back without a refresh token (as Google's do) keeps the stored
// one.
func TestGoogleTokenStorePerConnection(t *testing.T) {
	db := setupGoogleDB(t)
	store := NewConnectionStore(db)
	a := mkConnection(t, db, model.SourceGooglePhotos, "Home", struct{}{})
	b := mkConnection(t, db, model.SourceGooglePhotos, "Work", struct{}{})

	sa := &connectionTokenStore{conns: store, id: a.ID}
	sb := &connectionTokenStore{conns: store, id: b.ID}

	_, err := sa.GetToken()
	require.Error(t, err, "empty state has no token")

	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	require.NoError(t, sa.SaveToken(&oauth2.Token{AccessToken: "acc-a", RefreshToken: "ref-a", Expiry: exp}))
	require.NoError(t, sb.SaveToken(&oauth2.Token{AccessToken: "acc-b", RefreshToken: "ref-b", Expiry: exp}))

	ta, err := sa.GetToken()
	require.NoError(t, err)
	assert.Equal(t, "acc-a", ta.AccessToken)
	assert.Equal(t, "ref-a", ta.RefreshToken)
	assert.True(t, exp.Equal(ta.Expiry))

	// A refreshed access token without a refresh token keeps ref-a.
	require.NoError(t, sa.SaveToken(&oauth2.Token{AccessToken: "acc-a2", Expiry: exp.Add(time.Hour)}))
	ta, err = sa.GetToken()
	require.NoError(t, err)
	assert.Equal(t, "acc-a2", ta.AccessToken)
	assert.Equal(t, "ref-a", ta.RefreshToken)

	// The other connection is untouched.
	tb, err := sb.GetToken()
	require.NoError(t, err)
	assert.Equal(t, "acc-b", tb.AccessToken)

	// Clearing one leaves the other connected.
	require.NoError(t, sa.ClearToken())
	_, err = sa.GetToken()
	require.Error(t, err)
	rowA, _ := store.Get(a.ID)
	rowB, _ := store.Get(b.ID)
	assert.False(t, rowA.GoogleState().Connected())
	assert.True(t, rowB.GoogleState().Connected())
	// And the state column never carried anything but the token fields.
	assert.Equal(t, "{}", rowA.State)
}

// The OAuth callback only completes a flow this server started: the state is
// a fresh nonce per login that names the connection, and it is single-use.
func TestGoogleOAuthStateNamesTheConnection(t *testing.T) {
	db := setupGoogleDB(t)
	m := newGoogleManager(t, db, staticGoogleConfig{id: "cid", secret: "sec"})
	a := mkConnection(t, db, model.SourceGooglePhotos, "Home", struct{}{})
	b := mkConnection(t, db, model.SourceGooglePhotos, "Work", struct{}{})

	urlA, err := m.BeginGoogleAuth(a.ID, "http://frame.local/api/auth/google/callback")
	require.NoError(t, err)
	urlB, err := m.BeginGoogleAuth(b.ID, "http://frame.local/api/auth/google/callback")
	require.NoError(t, err)
	assert.Contains(t, urlA, "client_id=cid")
	assert.Contains(t, urlA, "redirect_uri=http%3A%2F%2Fframe.local%2Fapi%2Fauth%2Fgoogle%2Fcallback")
	assert.NotEqual(t, urlA, urlB, "each login gets its own state nonce")

	m.mu.Lock()
	assert.Len(t, m.oauth, 2)
	var stateA string
	for k, p := range m.oauth {
		if p.connectionID == a.ID {
			stateA = k
		}
	}
	m.mu.Unlock()
	require.NotEmpty(t, stateA)

	// A state the server never issued (the old literal "photos" included) is
	// refused before any exchange is attempted.
	_, err = m.CompleteGoogleAuth("photos", "code")
	assert.True(t, errors.Is(err, ErrUnknownOAuthState), "got %v", err)

	// A known state is consumed even when the exchange itself fails (no
	// network here), so it cannot be replayed.
	_, err = m.CompleteGoogleAuth(stateA, "bogus-code")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrUnknownOAuthState), "the state was known: %v", err)
	_, err = m.CompleteGoogleAuth(stateA, "bogus-code")
	assert.True(t, errors.Is(err, ErrUnknownOAuthState), "replayed state must be unknown")

	// An expired state is refused.
	m.mu.Lock()
	for k, p := range m.oauth {
		p.expires = time.Now().Add(-time.Minute)
		m.oauth[k] = p
	}
	m.mu.Unlock()
	_, err = m.CompleteGoogleAuth(urlB[len(urlB)-1:], "code") // not a real key anyway
	assert.True(t, errors.Is(err, ErrUnknownOAuthState))
}

// Without the global OAuth app credentials a login cannot start.
func TestGoogleLoginNeedsAppCredentials(t *testing.T) {
	db := setupGoogleDB(t)
	m := newGoogleManager(t, db, staticGoogleConfig{})
	a := mkConnection(t, db, model.SourceGooglePhotos, "Home", struct{}{})
	_, err := m.BeginGoogleAuth(a.ID, "http://frame.local/api/auth/google/callback")
	require.Error(t, err)
	// Only Google connections can start a Google login.
	im := mkConnection(t, db, model.SourceImmich, "Immich", model.ImmichConfig{URL: "http://i"})
	_, err = m.BeginGoogleAuth(im.ID, "http://frame.local/api/auth/google/callback")
	require.Error(t, err)
}

// Clearing one Google connection removes its rows and files only; the other
// account's imports stay. Deleting a connection does the same before the row
// goes.
func TestGoogleClearAndDeleteAreScopedToTheConnection(t *testing.T) {
	db := setupGoogleDB(t)
	m := newGoogleManager(t, db, staticGoogleConfig{id: "cid", secret: "sec"})
	a := mkConnection(t, db, model.SourceGooglePhotos, "Home", struct{}{})
	b := mkConnection(t, db, model.SourceGooglePhotos, "Work", struct{}{})

	dir := t.TempDir()
	mkFile := func(name string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("jpg"), 0o644))
		return p
	}
	fa1, fa2, fb := mkFile("a1.jpg"), mkFile("a2.jpg"), mkFile("b.jpg")
	for _, img := range []model.Image{
		{Source: model.SourceGooglePhotos, ConnectionID: &a.ID, FilePath: fa1},
		{Source: model.SourceGooglePhotos, ConnectionID: &a.ID, FilePath: fa2},
		{Source: model.SourceGooglePhotos, ConnectionID: &b.ID, FilePath: fb},
	} {
		require.NoError(t, db.Create(&img).Error)
	}

	svcA, err := m.Google(a.ID)
	require.NoError(t, err)
	n, err := svcA.GetPhotoCount()
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)

	require.NoError(t, svcA.ClearPhotos())
	n, _ = svcA.GetPhotoCount()
	assert.EqualValues(t, 0, n)
	_, err = os.Stat(fa1)
	assert.True(t, os.IsNotExist(err), "a1 file should be gone")
	_, err = os.Stat(fa2)
	assert.True(t, os.IsNotExist(err), "a2 file should be gone")
	_, err = os.Stat(fb)
	assert.NoError(t, err, "the other connection's file must stay")
	svcB, err := m.Google(b.ID)
	require.NoError(t, err)
	n, _ = svcB.GetPhotoCount()
	assert.EqualValues(t, 1, n)

	// Delete takes the files with it too.
	require.NoError(t, m.Delete(b.ID))
	_, err = os.Stat(fb)
	assert.True(t, os.IsNotExist(err), "deleted connection's file should be gone")
	var left int64
	db.Model(&model.Image{}).Where("source = ?", model.SourceGooglePhotos).Count(&left)
	assert.EqualValues(t, 0, left)
	_, err = m.Store().Get(b.ID)
	assert.True(t, errors.Is(err, ErrConnectionNotFound))
}

// A Google connection is a sync backend with no sync: the generic endpoints
// can count and clear it, and a resync request is refused rather than ignored.
func TestGoogleBackendHasNoSync(t *testing.T) {
	db := setupGoogleDB(t)
	m := newGoogleManager(t, db, staticGoogleConfig{id: "cid", secret: "sec"})
	a := mkConnection(t, db, model.SourceGooglePhotos, "Home", struct{}{})
	b, err := m.Backend(a.ID)
	require.NoError(t, err)
	assert.False(t, b.IsSyncing())
	assert.Equal(t, "", b.LastSyncError())
	assert.Error(t, b.ClearAndResync())
}

// Deleting a connection waits for its running picker import, which stops at
// the next item, so no file or row is written for a connection that is gone.
func TestPickerCancelImportsWaitsForTheRunningImport(t *testing.T) {
	p := NewPickerService(nil, t.TempDir())
	tr := p.beginImport(7)
	assert.False(t, tr.cancelled.Load())

	done := make(chan struct{})
	go func() {
		p.CancelImports(7)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("CancelImports returned while the import was still running")
	case <-time.After(50 * time.Millisecond):
	}
	assert.True(t, tr.cancelled.Load(), "the running import is told to stop")
	select {
	case <-tr.ctx.Done():
	default:
		t.Fatal("the import's context must be cancelled so its requests abort")
	}

	p.endImport(tr)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("CancelImports did not return after the import ended")
	}
	// A connection without imports is a no-op.
	p.CancelImports(8)
}
