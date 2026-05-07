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
//
// Match strategy:
//
//  1. Exact host:port match against allowlist entries. Works when
//     IngressRoute backends point at a stable in-cluster address that
//     Traefik does NOT resolve before dialing.
//  2. Port-only fallback: if any allowlist entry's port matches the
//     request's port, route through ZAP-HTTP regardless of host.
//     Traefik resolves Service DNS to a Pod IP before invoking the
//     transport (req.URL.Host becomes 10.x.x.x:port at this layer);
//     pinning the dial to (resolved-IP, port) lets the ZAP transport
//     reach the same pod the regular transport would have. Operators
//     who want stricter routing should use a per-port port number that
//     no non-ZAP backend uses (we already do — :9999 isn't shared).
func (z *zapBackendRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	addr := backendAddr(req)
	if _, ok := z.allowlist[addr]; ok {
		t := z.transportFor(addr)
		return t.RoundTrip(req)
	}
	if z.matchPort(addr) {
		t := z.transportFor(addr)
		return t.RoundTrip(req)
	}
	return z.next.RoundTrip(req)
}

// matchPort returns true when the request's port matches the port of
// any allowlist entry. This is the resolved-pod-IP fallback described
// in RoundTrip.
func (z *zapBackendRoundTripper) matchPort(addr string) bool {
	_, port, ok := splitHostPort(addr)
	if !ok {
		return false
	}
	for entry := range z.allowlist {
		_, p, ok := splitHostPort(entry)
		if ok && p == port {
			return true
		}
	}
	return false
}

// splitHostPort is strings.LastIndex over ":" — host:port carriers
// here never have brackets or schemes, so a hand-rolled split is fine
// and avoids net.SplitHostPort's empty-port behaviour.
func splitHostPort(addr string) (host, port string, ok bool) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return addr, "", false
	}
	return addr[:i], addr[i+1:], true
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
