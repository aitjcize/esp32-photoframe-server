package photoframe

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Every request reports a non-200 answer as a StatusError, so a caller can
// tell a refused password apart whichever request hit it.
func TestEveryRequestReturnsStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"authentication required"}`))
	}))
	defer srv.Close()
	c := NewClient(strings.TrimPrefix(srv.URL, "http://"))

	calls := map[string]func() error{
		"FetchSystemInfo": func() error { _, err := c.FetchSystemInfo(); return err },
		"FetchConfig":     func() error { _, err := c.FetchConfig(); return err },
		"FetchProcessingSettings": func() error {
			_, err := c.FetchProcessingSettings()
			return err
		},
		"FetchPalette":           func() error { _, err := c.FetchPalette(); return err },
		"PushProcessingSettings": func() error { return c.PushProcessingSettings([]byte(`{}`)) },
		"PushConfig":             func() error { return c.PushConfig(map[string]interface{}{}) },
		"PatchConfig":            func() error { return c.PatchConfig(map[string]interface{}{}) },
		"PushImage":              func() error { return c.PushImage([]byte("img"), nil) },
	}
	for name, call := range calls {
		err := call()
		var se *StatusError
		if !errors.As(err, &se) || se.StatusCode != 401 || se.Message != "authentication required" {
			t.Errorf("%s: error = %#v, want a 401 StatusError", name, err)
		}
		if !strings.Contains(err.Error(), "device returned status: 401") {
			t.Errorf("%s: Error() = %q, want the usual wording", name, err.Error())
		}
	}
}

func TestWithStatusObserverSeesEveryAnswer(t *testing.T) {
	var status int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")

	for name, c := range map[string]*Client{
		"open":       NewClient(host),
		"protected":  NewClientWithPassword(host, "pw"),
		"observer 2": NewClientWithPassword(host, "pw").WithStatusObserver(func(Answer) {}),
	} {
		t.Run(name, func(t *testing.T) {
			var seen []Answer
			c = c.WithStatusObserver(func(a Answer) { seen = append(seen, a) })
			before := time.Now()
			status = http.StatusUnauthorized
			_, _ = c.FetchConfig()
			status = http.StatusOK
			_ = c.PushConfig(map[string]interface{}{})
			after := time.Now()
			if len(seen) != 2 || seen[0].Status != 401 || seen[1].Status != 200 {
				t.Fatalf("observer saw %v, want statuses [401 200]", seen)
			}
			for _, a := range seen {
				if a.SentAt.Before(before) || a.ReceivedAt.Before(a.SentAt) || a.ReceivedAt.After(after) {
					t.Fatalf("answer times %v out of order with the request", a)
				}
			}
		})
	}
}

func TestWithStatusObserverKeepsPasswordAndRedirectPolicy(t *testing.T) {
	var gotPass string
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, leaked = r.BasicAuth()
	}))
	defer other.Close()
	frame := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotPass, _ = r.BasicAuth()
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer frame.Close()

	var seen []int
	c := NewClientWithPassword(strings.TrimPrefix(frame.URL, "http://"), "s3cret").
		WithStatusObserver(func(a Answer) { seen = append(seen, a.Status) })
	_, err := c.FetchConfig()
	var se *StatusError
	if !errors.As(err, &se) || se.StatusCode != http.StatusFound {
		t.Fatalf("error = %#v, want the unfollowed 302", err)
	}
	if gotPass != "s3cret" {
		t.Fatalf("password = %q, want it still sent", gotPass)
	}
	if leaked {
		t.Fatal("password was sent to the redirect target")
	}
	if len(seen) != 1 || seen[0] != http.StatusFound {
		t.Fatalf("observer saw %v, want [302]", seen)
	}
}

func TestWithStatusObserverIsNotCalledWithoutAnAnswer(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	called := false
	c := NewClient(strings.TrimPrefix(dead.URL, "http://")).WithStatusObserver(func(Answer) { called = true })
	if _, err := c.FetchConfig(); err == nil {
		t.Fatal("expected a connection error")
	}
	if called {
		t.Fatal("observer called although the frame never answered")
	}
}

// A frame that takes the connection and then says nothing holds a bounded
// client for its bound, not the shared client's two minutes -- which stay
// the shared client's.
func TestWithTimeoutBoundsARequest(t *testing.T) {
	stall := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-stall
	}))
	t.Cleanup(func() {
		close(stall)
		srv.Close()
	})

	c := NewClientWithPassword(strings.TrimPrefix(srv.URL, "http://"), "pw").WithTimeout(100 * time.Millisecond)
	start := time.Now()
	_, err := c.FetchConfig()
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("request took %v, want it bounded", took)
	}
	if sharedHTTPClient.Timeout != 120*time.Second {
		t.Fatalf("shared client timeout changed to %v", sharedHTTPClient.Timeout)
	}
}
