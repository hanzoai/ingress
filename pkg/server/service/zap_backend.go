// Package service: zap_backend wires a ZAP-HTTP transport for selected
// upstream backends. When a backend's host:port matches the configured
// allowlist, requests are dialed via github.com/zap-proto/http instead of
// net/http. All other backends fall through to the wrapped RoundTripper
// unchanged. External (client-facing) TLS is unaffected.
//
// Configuration: env var INGRESS_ZAP_BACKENDS holds a comma-separated
// list of host:port targets, e.g.
//
//   INGRESS_ZAP_BACKENDS=svc1.ns.svc:8080,svc2.ns.svc:8080
//
// IngressRoute manifests opt in by pointing the service at one of those
// host:port targets; no schema change is required. See
// docs/content/reference/dynamic-configuration/zap-backend.md for an
// example manifest.
package service

import (
	"net/http"
	"os"
	"strings"
	"sync"

	zaphttp "github.com/zap-proto/http"
)

// zapBackendEnv is the env var that lists ZAP-HTTP backend host:port
// targets, comma-separated.
const zapBackendEnv = "INGRESS_ZAP_BACKENDS"

// zapBackendRoundTripper routes requests to a ZAP-HTTP transport when
// the request's destination host:port matches the allowlist; everything
// else is delegated to the wrapped RoundTripper.
type zapBackendRoundTripper struct {
	next       http.RoundTripper
	allowlist  map[string]struct{}
	transports sync.Map // host:port -> *zaphttp.Transport
}

// newZapBackendRoundTripper wraps next. If the env var is empty, it
// returns next unchanged so the call site stays oblivious.
func newZapBackendRoundTripper(next http.RoundTripper) http.RoundTripper {
	addrs := parseZapBackendList(os.Getenv(zapBackendEnv))
	if len(addrs) == 0 {
		return next
	}
	allow := make(map[string]struct{}, len(addrs))
	for _, a := range addrs {
		allow[a] = struct{}{}
	}
	return &zapBackendRoundTripper{next: next, allowlist: allow}
}

// parseZapBackendList splits a comma-separated list and trims whitespace.
// Empty entries are dropped.
func parseZapBackendList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// RoundTrip dispatches to ZAP-HTTP if the destination is in the
// allowlist, otherwise to the wrapped transport.
func (z *zapBackendRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	addr := backendAddr(req)
	if _, ok := z.allowlist[addr]; !ok {
		return z.next.RoundTrip(req)
	}
	t := z.transportFor(addr)
	return t.RoundTrip(req)
}

// transportFor returns a per-addr ZAP transport, lazily created.
// zaphttp.Transport pins to a single peer (it ignores req.URL.Host),
// so we keep one per backend address.
func (z *zapBackendRoundTripper) transportFor(addr string) *zaphttp.Transport {
	if v, ok := z.transports.Load(addr); ok {
		return v.(*zaphttp.Transport)
	}
	t := zaphttp.NewTransport(addr)
	actual, _ := z.transports.LoadOrStore(addr, t)
	return actual.(*zaphttp.Transport)
}

// backendAddr returns the upstream host:port for the request. Traefik
// rewrites req.URL to point at the chosen backend before invoking the
// transport, so URL.Host is the source of truth. Falls back to req.Host
// if URL.Host is empty (defensive; this is not expected in practice).
func backendAddr(req *http.Request) string {
	if req.URL != nil && req.URL.Host != "" {
		return req.URL.Host
	}
	return req.Host
}
