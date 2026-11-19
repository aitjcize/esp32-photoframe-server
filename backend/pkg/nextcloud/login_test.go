package nextcloud

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStartAndPollLogin(t *testing.T) {
	granted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("%s %s, want POST", r.Method, r.URL.Path)
		}
		if r.UserAgent() != UserAgent {
			t.Errorf("User-Agent = %q", r.UserAgent())
		}
		switch r.URL.Path {
		case "/index.php/login/v2":
			// The endpoint Nextcloud names is ignored in favour of the base URL.
			writeJSON(w, map[string]any{
				"poll":  map[string]string{"token": "tok", "endpoint": "http://unreachable.invalid/login/v2/poll"},
				"login": "https://cloud.example.com/login/v2/flow/abc",
			})
		case "/index.php/login/v2/poll":
			_ = r.ParseForm()
			if r.PostForm.Get("token") != "tok" {
				t.Errorf("poll token = %q", r.PostForm.Get("token"))
			}
			if !granted {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			writeJSON(w, map[string]string{"server": "https://cloud.example.com", "loginName": "alice", "appPassword": "generated"})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	hc := &http.Client{Timeout: 5 * time.Second}

	flow, err := StartLogin(hc, srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if flow.LoginURL != "https://cloud.example.com/login/v2/flow/abc" || flow.Token != "tok" {
		t.Fatalf("flow = %+v", flow)
	}

	creds, err := PollLogin(hc, srv.URL, flow.Token)
	if err != nil || creds != nil {
		t.Fatalf("pending poll: creds=%v err=%v", creds, err)
	}
	granted = true
	creds, err = PollLogin(hc, srv.URL, flow.Token)
	if err != nil || creds == nil || creds.LoginName != "alice" || creds.AppPassword != "generated" {
		t.Fatalf("granted poll: creds=%+v err=%v", creds, err)
	}
}

func TestStartLoginRejectsNonNextcloud(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>not nextcloud</html>"))
	}))
	defer srv.Close()
	if _, err := StartLogin(&http.Client{}, srv.URL); err == nil {
		t.Fatal("expected an error for a non-Nextcloud response")
	}
}

func TestRevokeAppPassword(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		if r.Method != "DELETE" || r.URL.Path != "/ocs/v2.php/core/apppassword" || user != "alice" || pass != "app-pass" {
			t.Errorf("%s %s as %s/%s", r.Method, r.URL.Path, user, pass)
		}
		called = true
	}))
	defer srv.Close()
	if err := newTestClient(srv.URL).RevokeAppPassword(); err != nil || !called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}
