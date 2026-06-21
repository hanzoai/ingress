package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	zaplib "github.com/luxfi/zap"
	"github.com/luxfi/zap/forward"
)

// pickFreePort returns an OS-assigned free TCP port. Mirrors luxfi/zap's
// forward test helper so node ports never collide across parallel tests.
func pickFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// TestForwardBackendRoundTripper_AIRouteTunnelsOverZAP stands up the
// canonical Forward terminal (forward.Serve) on a backend ZAP node, points
// the edge forwardBackendRoundTripper at it (this is the gateway-relay peer
// in the test), and proves that a matching AI request:
//
//   - is packaged into a Forward envelope and Call'd over ZAP (the response
//     carries X-Backend=zap, set by the terminal handler, and the round-trip
//     status/body are intact);
//   - keeps its Authorization Bearer JWT across the hop (the gateway-relay's
//     auth path), while
//   - a client-spoofed identity header (X-Org-Id) is STRIPPED on this leg —
//     the edge→gateway client leg never lets the client assert identity.
//
// A non-matching route is served by the wrapped (next) transport, untouched.
func TestForwardBackendRoundTripper_AIRouteTunnelsOverZAP(t *testing.T) {
	bePort := pickFreePort(t)
	backend := zaplib.NewNode(zaplib.NodeConfig{NodeID: "gateway-relay", Port: bePort, NoDiscovery: true})
	if err := backend.Start(); err != nil {
		t.Fatalf("backend.Start: %v", err)
	}
	t.Cleanup(backend.Stop)

	// Canonical terminal: decodes the Forward, reconstructs the *http.Request
	// (injecting edge identity, stripping client-smuggled identity), serves
	// it, returns the Response envelope. The handler echoes back what it saw
	// so the test can assert what survived the hop.
	forward.Serve(backend, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		out := map[string]string{
			"auth":   r.Header.Get("Authorization"),
			"org":    r.Header.Get(forward.HeaderOrgID),
			"method": r.Method,
			"path":   r.URL.Path,
			"query":  r.URL.RawQuery,
			"body":   string(body),
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Backend", "zap")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(out)
	}))

	// nextSentinel proves the wrapped transport is used for non-AI routes and
	// is NEVER touched for matched AI routes.
	var nextHit bool
	next := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		nextHit = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"X-Backend": {"http"}},
			Body:       io.NopCloser(bytes.NewReader([]byte("http-ok"))),
		}, nil
	})

	// Build the wrapper directly (bypass env parsing so the test does not race
	// other tests via the process env). The wrapper lazily creates its own
	// edge node and ConnectDirectID's the backend (the gateway-relay peer),
	// exercising the real peer()/dial()/cache path.
	rt := &forwardBackendRoundTripper{
		next:     next,
		gwAddr:   "127.0.0.1:" + strconv.Itoa(bePort),
		prefixes: []string{"/v1/chat", "/v1/completions", "/v1/models"},
	}

	// --- Matched AI route: must tunnel over ZAP to the terminal. ---
	body := `{"model":"zen","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest(http.MethodPost,
		"http://edge/v1/chat/completions?stream=true", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-jwt") // must SURVIVE the hop
	req.Header.Set(forward.HeaderOrgID, "evil-org")     // spoofed; must be STRIPPED

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := rt.RoundTrip(req.WithContext(ctx))
	if err != nil {
		t.Fatalf("AI-route RoundTrip: %v", err)
	}
	if nextHit {
		t.Fatal("matched AI route fell through to next transport; must tunnel over ZAP")
	}

	// The RoundTripper must NOT mutate the request it was given (the ingress may
	// reuse/retry it): stripEdgeIdentity clones, so the caller's request still
	// carries its original identity header here.
	if got := req.Header.Get(forward.HeaderOrgID); got != "evil-org" {
		t.Errorf("RoundTrip mutated caller's request headers: X-Org-Id=%q, want preserved on the original", got)
	}

	// Status + the terminal's marker header prove the response came back
	// through the ZAP hop, not net/http.
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("status: got %d want %d", resp.StatusCode, http.StatusAccepted)
	}
	if got := resp.Header.Get("X-Backend"); got != "zap" {
		t.Errorf("X-Backend: got %q want zap (response did not traverse ZAP)", got)
	}

	var echo map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&echo); err != nil {
		t.Fatalf("decode echo: %v", err)
	}
	_ = resp.Body.Close()

	// Body intact end-to-end.
	if echo["body"] != body {
		t.Errorf("body: got %q want %q", echo["body"], body)
	}
	if echo["method"] != http.MethodPost {
		t.Errorf("method: got %q want POST", echo["method"])
	}
	if echo["path"] != "/v1/chat/completions" {
		t.Errorf("path: got %q want /v1/chat/completions", echo["path"])
	}
	if echo["query"] != "stream=true" {
		t.Errorf("query: got %q want stream=true", echo["query"])
	}
	// INVARIANT (the security contract of this leg): Authorization survives,
	// client-asserted identity does NOT.
	//
	// Authorization must survive — the gateway-relay validates the forwarded
	// Bearer JWT; that is the real auth path.
	if echo["auth"] != "Bearer test-jwt" {
		t.Errorf("Authorization did not survive the hop: got %q want %q", echo["auth"], "Bearer test-jwt")
	}
	// Client-spoofed identity must be fully stripped on the edge→gateway leg.
	// The edge asserts NO validated identity (it is upstream of JWT
	// validation), so the terminal must see an EMPTY X-Org-Id — not the
	// spoofed "evil-org", and not any other value.
	if echo["org"] != "" {
		t.Errorf("client identity leaked through the Forward leg: X-Org-Id=%q, want empty", echo["org"])
	}

	// --- Non-matching route: must use the wrapped (next) transport. ---
	nextHit = false
	req2, _ := http.NewRequest(http.MethodGet, "http://edge/v1/other/thing", nil)
	resp2, err := rt.RoundTrip(req2)
	if err != nil {
		t.Fatalf("non-AI-route RoundTrip: %v", err)
	}
	if !nextHit {
		t.Fatal("non-AI route did not use next transport")
	}
	if got := resp2.Header.Get("X-Backend"); got != "http" {
		t.Errorf("non-AI route X-Backend: got %q want http", got)
	}
	b2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()
	if string(b2) != "http-ok" {
		t.Errorf("non-AI body: got %q want http-ok", string(b2))
	}
}

// TestForwardBackend_DefaultOff proves the additive-only guarantee: with
// neither env set, both the per-leg constructor and the composed edge seam
// return the wrapped transport UNCHANGED (zero behavior change, opt-in). The
// sentinel is a pointer type so interface equality is exact identity — the
// constructors must return the very value passed in, not a wrapper.
func TestForwardBackend_DefaultOff(t *testing.T) {
	next := &sentinelRT{}

	t.Run("forward leg constructor, empty env", func(t *testing.T) {
		t.Setenv(forwardZapAddrEnv, "")
		t.Setenv(forwardRoutesEnv, "")
		if got := newForwardBackendRoundTripper(next); got != next {
			t.Fatal("empty env must return next unchanged (forward leg disabled)")
		}
	})

	t.Run("addr set but no routes ⇒ inert", func(t *testing.T) {
		t.Setenv(forwardZapAddrEnv, "gateway.hanzo.svc:9999")
		t.Setenv(forwardRoutesEnv, "")
		if got := newForwardBackendRoundTripper(next); got != next {
			t.Fatal("addr without routes must return next unchanged (no AI route can match)")
		}
	})

	t.Run("routes set but no addr ⇒ inert", func(t *testing.T) {
		t.Setenv(forwardZapAddrEnv, "")
		t.Setenv(forwardRoutesEnv, "/v1/chat")
		if got := newForwardBackendRoundTripper(next); got != next {
			t.Fatal("routes without addr must return next unchanged (no relay to dial)")
		}
	})

	t.Run("composed edge seam, both env empty", func(t *testing.T) {
		// With both ZAP legs' env empty, newZapEdgeRoundTripper must be a pure
		// pass-through: forward leg returns next, zap-backend leg returns next.
		t.Setenv(forwardZapAddrEnv, "")
		t.Setenv(forwardRoutesEnv, "")
		t.Setenv(zapBackendEnv, "")
		if got := newZapEdgeRoundTripper(next); got != next {
			t.Fatal("both ZAP legs disabled: edge seam must return next unchanged")
		}
	})
}

// TestParseForwardRoutes exercises the comma-separated prefix parser.
func TestParseForwardRoutes(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"/v1/chat", []string{"/v1/chat"}},
		{"/v1/chat,/v1/models", []string{"/v1/chat", "/v1/models"}},
		{" /v1/chat , /v1/models ,, ", []string{"/v1/chat", "/v1/models"}},
	}
	for _, tc := range cases {
		got := parseForwardRoutes(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("parse(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("parse(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

// roundTripFunc adapts a func to http.RoundTripper for the active-path test
// next hop (where the func body flips a sentinel; never compared with ==).
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// sentinelRT is a comparable (pointer) RoundTripper used by the default-off
// tests: the constructors must return the EXACT value passed in when
// disabled, so interface == is pointer identity. It is never invoked.
type sentinelRT struct{}

func (*sentinelRT) RoundTrip(*http.Request) (*http.Response, error) {
	panic("sentinelRT.RoundTrip must never be called (disabled path)")
}
