package photoframe

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClientWithPasswordSendsBasicAuth(t *testing.T) {
	var gotPass string
	var gotOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotPass, gotOK = r.BasicAuth()
	}))
	defer srv.Close()

	c := NewClientWithPassword("frame.local", "s3cret")
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !gotOK || gotPass != "s3cret" {
		t.Fatalf("basic auth = (%q, %v), want (\"s3cret\", true)", gotPass, gotOK)
	}
	if req.Header.Get("Authorization") != "" {
		t.Fatal("caller's request was mutated")
	}
}

func TestNewClientWithoutPasswordUsesSharedClient(t *testing.T) {
	if NewClientWithPassword("frame.local", "").httpClient != sharedHTTPClient {
		t.Fatal("empty password should reuse the shared client")
	}
}

func TestPasswordNotForwardedOnRedirect(t *testing.T) {
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, leaked = r.BasicAuth()
	}))
	defer other.Close()
	frame := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer frame.Close()

	c := NewClientWithPassword("frame.local", "s3cret")
	req, _ := http.NewRequest("GET", frame.URL, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want the unfollowed 302", resp.StatusCode)
	}
	if leaked {
		t.Fatal("password was sent to the redirect target")
	}
}

func TestPatchConfigSendsPartialUpdateWithCurrentPassword(t *testing.T) {
	var gotMethod, gotPass, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_, gotPass, _ = r.BasicAuth()
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	c := NewClientWithPassword(strings.TrimPrefix(srv.URL, "http://"), "current")
	if err := c.PatchConfig(map[string]interface{}{"http_password": "next"}); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PATCH" || gotPass != "current" || gotBody != `{"http_password":"next"}` {
		t.Fatalf("got %s with password %q and body %s", gotMethod, gotPass, gotBody)
	}
}

func TestStatusErrorCarriesRetryAfterAndMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			w.Header().Set("Retry-After", "42")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"too many wrong passwords, try again later"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":"error","message":"Device password is too long (max 63 bytes)"}`))
	}))
	defer srv.Close()
	c := NewClient(strings.TrimPrefix(srv.URL, "http://"))

	var se *StatusError
	err := c.PatchConfig(map[string]interface{}{"http_password": "x"})
	if !errors.As(err, &se) || se.StatusCode != 429 || se.RetryAfter != "42" ||
		se.Message != "too many wrong passwords, try again later" {
		t.Fatalf("PatchConfig error = %#v", err)
	}
	if err.Error() != "device returned status: 429" {
		t.Fatalf("Error() = %q, want the usual wording", err.Error())
	}

	_, err = c.FetchConfig()
	if !errors.As(err, &se) || se.StatusCode != 400 || se.Message != "Device password is too long (max 63 bytes)" {
		t.Fatalf("FetchConfig error = %#v", err)
	}
}

func TestResolveHostKeepsPort(t *testing.T) {
	for host, want := range map[string]string{
		"127.0.0.1":      "127.0.0.1",
		"127.0.0.1:8080": "127.0.0.1:8080",
		"[::1]:8080":     "[::1]:8080",
		"::1":            "[::1]:80",
	} {
		got, err := NewClient(host).resolveHost(host)
		if err != nil || got != want {
			t.Errorf("resolveHost(%q) = %q, %v; want %q", host, got, err, want)
		}
	}
}

// The Supervisor's mDNS resolver can answer a .local name with only the
// frame's IPv6 addresses; an IPv4 is retried for, a routable IPv6 is used
// bracketed, and a link-local one is refused with a usable message.
func TestLookupIPRetriesForIPv4(t *testing.T) {
	lookupRetryDelay = 0
	defer func() { lookupHost, lookupRetryDelay = net.LookupHost, 500*time.Millisecond }()

	const ula, linkLocal = "fdb6:1af3:9acd:d71c:9270:69ff:fe11:5790", "fe80::9270:69ff:fe11:5790"
	for name, tc := range map[string]struct {
		answers [][]string
		want    string
		calls   int
		errHint string
	}{
		"ipv4 on the third try": {
			answers: [][]string{{linkLocal, ula}, {ula}, {ula, "192.168.0.160"}},
			want:    "192.168.0.160", calls: 3,
		},
		"ipv4 first try": {
			answers: [][]string{{"192.168.0.160", ula}},
			want:    "192.168.0.160", calls: 1,
		},
		"routable ipv6 only": {
			answers: [][]string{{linkLocal, ula}, {linkLocal, ula}, {linkLocal, ula}},
			want:    "[" + ula + "]:80", calls: 3,
		},
		"link-local only": {
			answers: [][]string{{linkLocal}, {linkLocal}, {linkLocal}},
			calls:   3, errHint: "IPv4 address",
		},
		"error then ipv4": {
			answers: [][]string{nil, {"192.168.0.160"}},
			want:    "192.168.0.160", calls: 2,
		},
		"error every time": {
			answers: [][]string{nil, nil, nil},
			calls:   3, errHint: "no such host",
		},
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			lookupHost = func(string) ([]string, error) {
				a := tc.answers[calls]
				calls++
				if a == nil {
					return nil, errors.New("lookup frame.local: no such host")
				}
				return a, nil
			}
			got, err := NewClient("frame.local").resolveHost("frame.local")
			if calls != tc.calls {
				t.Errorf("lookups = %d, want %d", calls, tc.calls)
			}
			if tc.errHint != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errHint) {
					t.Fatalf("resolveHost = %q, %v; want error containing %q", got, err, tc.errHint)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("resolveHost = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestCheckReachabilityIPv6(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.NotFoundHandler())
	l, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("no IPv6 loopback")
	}
	srv.Listener = l
	srv.Start()
	defer srv.Close()
	if err := NewClient("").checkReachability(l.Addr().String()); err != nil {
		t.Fatalf("checkReachability(%s): %v", l.Addr(), err)
	}
}
