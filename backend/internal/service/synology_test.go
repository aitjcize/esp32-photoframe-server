package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// newFakeSynology returns a test server whose auth.cgi responds per succeed,
// and a counter of login attempts. Each successful login issues a unique SID.
func newFakeSynology(succeed bool) (*httptest.Server, *int32) {
	var authCalls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "auth.cgi") {
			n := atomic.AddInt32(&authCalls, 1)
			if succeed {
				fmt.Fprintf(w, `{"success":true,"data":{"sid":"sid-%d","synotoken":"tok-%d"}}`, n, n)
			} else {
				fmt.Fprint(w, `{"success":false,"error":{"code":400}}`)
			}
			return
		}
		fmt.Fprint(w, `{"success":true}`)
	}))
	return ts, &authCalls
}

func newTestSynologyService(t *testing.T, serverURL string) *SynologyService {
	t.Helper()
	db := setupGateDB(t)
	c := mkConnection(t, db, model.SourceSynologyPhotos, "NAS", model.SynologyConfig{
		URL: serverURL, Account: "user", Password: "pass",
	})
	return NewSynologyService(db, NewConnectionStore(db), c.ID)
}

// setSynologyURL rewrites the service's connection URL, as the Settings UI does.
func setSynologyURL(t *testing.T, svc *SynologyService, url string) {
	t.Helper()
	c, err := svc.conns.Get(svc.id)
	require.NoError(t, err)
	cfg := c.SynologyConfig()
	cfg.URL = url
	require.NoError(t, c.EncodeConfig(cfg))
	require.NoError(t, svc.conns.Update(&c))
}

// A failed login must start a cooldown during which background callers
// (thumbnail proxying, auto-sync) fail fast instead of serially re-attempting
// the login under the service mutex — that serialization used to block the
// explicit Connect request for minutes when a NAS session expired.
func TestSynologyLoginCooldown(t *testing.T) {
	ts, authCalls := newFakeSynology(false)
	defer ts.Close()
	svc := newTestSynologyService(t, ts.URL)

	// First background attempt reaches the server and fails.
	err := mustLogin(svc, "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "login failed")
	assert.EqualValues(t, 1, atomic.LoadInt32(authCalls))

	// During the cooldown, background attempts fail fast with no network call.
	err = mustLogin(svc, "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recently failed")
	assert.EqualValues(t, 1, atomic.LoadInt32(authCalls))

	// An explicit user action (Connect / Test) bypasses the cooldown.
	err = mustLogin(svc, "", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "login failed")
	assert.EqualValues(t, 2, atomic.LoadInt32(authCalls))
}

// Concurrent requests that all observed the same expired session must share a
// single re-login: once one succeeds, relogin calls carrying the stale
// generation are no-ops.
func TestSynologyReloginSharedAcrossRequests(t *testing.T) {
	ts, authCalls := newFakeSynology(true)
	defer ts.Close()
	svc := newTestSynologyService(t, ts.URL)

	require.NoError(t, mustLogin(svc, "", false))
	assert.EqualValues(t, 1, atomic.LoadInt32(authCalls))

	// Simulate N requests observing the session before it expired.
	gen := svc.loginGeneration()

	// The first one to hit auth-expired re-logs in.
	require.NoError(t, mustRelogin(svc, gen))
	assert.EqualValues(t, 2, atomic.LoadInt32(authCalls))

	// The rest see the newer session and skip the login entirely.
	require.NoError(t, mustRelogin(svc, gen))
	require.NoError(t, mustRelogin(svc, gen))
	assert.EqualValues(t, 2, atomic.LoadInt32(authCalls))

	// A request that observed the refreshed session re-logs in for real.
	require.NoError(t, mustRelogin(svc, svc.loginGeneration()))
	assert.EqualValues(t, 3, atomic.LoadInt32(authCalls))
}

// A successful login must clear the failure cooldown.
func TestSynologyLoginSuccessClearsCooldown(t *testing.T) {
	tsFail, _ := newFakeSynology(false)
	defer tsFail.Close()
	tsOK, okCalls := newFakeSynology(true)
	defer tsOK.Close()

	svc := newTestSynologyService(t, tsFail.URL)
	require.Error(t, mustLogin(svc, "", false)) // starts cooldown

	// User fixes the URL and explicitly connects.
	setSynologyURL(t, svc, tsOK.URL)
	require.NoError(t, mustLogin(svc, "", true))
	assert.EqualValues(t, 1, atomic.LoadInt32(okCalls))

	// Background calls work again immediately (SID cached, no cooldown error).
	require.NoError(t, mustLogin(svc, "", false))
}

// mustLogin is ensureClient for tests that only care whether the login worked.
func mustLogin(svc *SynologyService, otp string, force bool) error {
	_, err := svc.ensureClient(otp, force)
	return err
}

// mustRelogin is relogin for tests that only care whether it worked.
func mustRelogin(svc *SynologyService, gen uint64) error {
	_, err := svc.relogin(gen)
	return err
}
