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
	db := setupTestDB()
	settings := NewSettingsService(db)
	require.NoError(t, settings.Set("synology_url", serverURL))
	require.NoError(t, settings.Set("synology_account", "user"))
	require.NoError(t, settings.Set("synology_password", "pass"))
	// Clear state a previous test may have left in the shared in-memory DB.
	require.NoError(t, settings.Set("synology_sid", ""))
	require.NoError(t, settings.Set("synology_did", ""))
	return NewSynologyService(db, settings)
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
	err := svc.ensureClient("", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "login failed")
	assert.EqualValues(t, 1, atomic.LoadInt32(authCalls))

	// During the cooldown, background attempts fail fast with no network call.
	err = svc.ensureClient("", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recently failed")
	assert.EqualValues(t, 1, atomic.LoadInt32(authCalls))

	// An explicit user action (Connect / Test) bypasses the cooldown.
	err = svc.ensureClient("", true)
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

	require.NoError(t, svc.ensureClient("", false))
	assert.EqualValues(t, 1, atomic.LoadInt32(authCalls))

	// Simulate N requests observing the session before it expired.
	gen := svc.loginGeneration()

	// The first one to hit auth-expired re-logs in.
	require.NoError(t, svc.relogin(gen))
	assert.EqualValues(t, 2, atomic.LoadInt32(authCalls))

	// The rest see the newer session and skip the login entirely.
	require.NoError(t, svc.relogin(gen))
	require.NoError(t, svc.relogin(gen))
	assert.EqualValues(t, 2, atomic.LoadInt32(authCalls))

	// A request that observed the refreshed session re-logs in for real.
	require.NoError(t, svc.relogin(svc.loginGeneration()))
	assert.EqualValues(t, 3, atomic.LoadInt32(authCalls))
}

// A successful login must clear the failure cooldown.
func TestSynologyLoginSuccessClearsCooldown(t *testing.T) {
	tsFail, _ := newFakeSynology(false)
	defer tsFail.Close()
	tsOK, okCalls := newFakeSynology(true)
	defer tsOK.Close()

	svc := newTestSynologyService(t, tsFail.URL)
	require.Error(t, svc.ensureClient("", false)) // starts cooldown

	// User fixes the URL and explicitly connects.
	require.NoError(t, svc.settings.Set("synology_url", tsOK.URL))
	require.NoError(t, svc.ensureClient("", true))
	assert.EqualValues(t, 1, atomic.LoadInt32(okCalls))

	// Background calls work again immediately (SID cached, no cooldown error).
	require.NoError(t, svc.ensureClient("", false))
}
