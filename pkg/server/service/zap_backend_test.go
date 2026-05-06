package service

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	zaphttp "github.com/zap-proto/http"
)

// TestZapBackendRoundTripper_RoutesAllowlistedHostToZap stands up a real
// ZAP-HTTP server, marks its host:port as a ZAP backend in the
// allowlist, and verifies a request whose URL.Host matches gets dialed
// over ZAP-HTTP. A second backend not in the allowlist is served by an
// httptest HTTP server and must keep using the wrapped transport.
func TestZapBackendRoundTripper_RoutesAllowlistedHostToZap(t *testing.T) {
	// 1. Spin up the ZAP-HTTP backend.
	zapLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer zapLn.Close()
	zapAddr := zapLn.Addr().String()

	zapMux := http.NewServeMux()
	zapMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "zap")
		_, _ = io.WriteString(w, "zap-ok")
	})
	zapSrv := &zaphttp.Server{Handler: zapMux, ReadTimeout: 5 * time.Second}
	go func() { _ = zapSrv.Serve(zapLn) }()
	defer zapSrv.Close()

	// 2. Spin up the plain-HTTP backend (must not be touched by ZAP).
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "http")
		_, _ = io.WriteString(w, "http-ok")
	}))
	defer httpSrv.Close()
	httpAddr := strings.TrimPrefix(httpSrv.URL, "http://")

	// 3. Build the wrapper directly. We bypass env-var parsing so the
	//    test does not race with other tests in the same package.
	rt := &zapBackendRoundTripper{
		next:      http.DefaultTransport,
		allowlist: map[string]struct{}{zapAddr: {}},
	}

	// 4. Allowlisted host: must reach ZAP backend.
	req, err := http.NewRequest(http.MethodGet, "http://"+zapAddr+"/healthz", nil)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("zap roundtrip: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if got := string(body); got != "zap-ok" {
		t.Fatalf("zap body = %q, want zap-ok", got)
	}
	if got := resp.Header.Get("X-Backend"); got != "zap" {
		t.Fatalf("zap header = %q, want zap", got)
	}

	// 5. Non-allowlisted host: must fall through to wrapped transport.
	req2, err := http.NewRequest(http.MethodGet, "http://"+httpAddr+"/anything", nil)
	if err != nil {
		t.Fatalf("new req2: %v", err)
	}
	resp2, err := rt.RoundTrip(req2)
	if err != nil {
		t.Fatalf("http roundtrip: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if got := string(body2); got != "http-ok" {
		t.Fatalf("http body = %q, want http-ok", got)
	}
	if got := resp2.Header.Get("X-Backend"); got != "http" {
		t.Fatalf("http header = %q, want http", got)
	}
}

// TestNewZapBackendRoundTripper_EmptyEnvReturnsNext: when no env var is
// set, the constructor returns the wrapped transport unchanged. This is
// the additive-only guarantee — opt-out by default.
func TestNewZapBackendRoundTripper_EmptyEnvReturnsNext(t *testing.T) {
	t.Setenv(zapBackendEnv, "")
	next := http.DefaultTransport
	got := newZapBackendRoundTripper(next)
	if got != next {
		t.Fatalf("with empty env, expected wrapped == next; got wrapper")
	}
}

// TestParseZapBackendList exercises the comma-separated env parser.
func TestParseZapBackendList(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a:1", []string{"a:1"}},
		{"a:1,b:2", []string{"a:1", "b:2"}},
		{" a:1 , b:2 ,, ", []string{"a:1", "b:2"}},
	}
	for _, tt := range tests {
		got := parseZapBackendList(tt.in)
		if len(got) != len(tt.want) {
			t.Fatalf("parse(%q) = %v, want %v", tt.in, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("parse(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
			}
		}
	}
}
