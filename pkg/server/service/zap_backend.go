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
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/valyala/fasthttp"
	zaphttp "github.com/zap-proto/http"

	"github.com/rs/zerolog/log"
)

// zapHTTPProto is the fallback protocol label used when a decoded frame
// carries none. It matches github.com/zap-proto/http's own defaultProto so
// the reconstructed *http.Response is byte-for-byte what the previous
// net/http ZAP-HTTP codec produced.
const zapHTTPProto = "ZAP-HTTP/1.0"

// zapBackendEnv is the env var that lists ZAP-HTTP backend host:port
// targets, comma-separated.
const zapBackendEnv = "INGRESS_ZAP_BACKENDS"

// zapMaxBodyEnv overrides the per-request body cap (bytes) the ZAP-HTTP codec
// buffers. Defaults to defaultMaxZapBodyBytes.
const zapMaxBodyEnv = "INGRESS_ZAP_MAX_BODY_BYTES"

// defaultMaxZapBodyBytes bounds the request body the ZAP-HTTP codec buffers in
// one frame. Without a cap, io.ReadAll on the request body lets a single
// oversized request OOM the ingress. 64 MiB is generous for API backends.
const defaultMaxZapBodyBytes int64 = 64 << 20

// zapBackendRoundTripper routes requests to a ZAP-HTTP transport when
// the request's destination host:port matches the allowlist; everything
// else is delegated to the wrapped RoundTripper.
type zapBackendRoundTripper struct {
	next       http.RoundTripper
	allowlist  map[string]struct{}
	maxBody    int64
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
	return &zapBackendRoundTripper{next: next, allowlist: allow, maxBody: parseMaxBody(os.Getenv(zapMaxBodyEnv))}
}

// parseMaxBody parses the body-cap env; falls back to the default on empty or
// invalid input. A non-positive value is rejected (a zero cap would break all
// bodied requests).
func parseMaxBody(raw string) int64 {
	if raw != "" {
		if n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return defaultMaxZapBodyBytes
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
//     the ingress does NOT resolve before dialing.
//  2. Port-only fallback: if any allowlist entry's port matches the
//     request's port, route through ZAP-HTTP regardless of host.
//     The ingress resolves Service DNS to a Pod IP before invoking the
//     transport (req.URL.Host becomes 10.x.x.x:port at this layer);
//     pinning the dial to (resolved-IP, port) lets the ZAP transport
//     reach the same pod the regular transport would have. Operators
//     who want stricter routing should use a per-port port number that
//     no non-ZAP backend uses (we already do — :9999 isn't shared).
func (z *zapBackendRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	addr := backendAddr(req)
	if _, ok := z.allowlist[addr]; ok {
		log.Info().Str("addr", addr).Str("path", req.URL.Path).
			Msg("[ZAP-BACKEND] dialing via zap-http (exact match)")
		t := z.transportFor(addr)
		return z.dialZap(t, req)
	}
	if z.matchPort(addr) {
		log.Info().Str("addr", addr).Str("path", req.URL.Path).
			Msg("[ZAP-BACKEND] dialing via zap-http (port match)")
		t := z.transportFor(addr)
		return z.dialZap(t, req)
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
	t := zaphttp.Dial("tcp", addr)
	actual, _ := z.transports.LoadOrStore(addr, t)
	return actual.(*zaphttp.Transport)
}

// backendAddr returns the upstream host:port for the request. The ingress
// rewrites req.URL to point at the chosen backend before invoking the
// transport, so URL.Host is the source of truth. Falls back to req.Host
// if URL.Host is empty (defensive; this is not expected in practice).
func backendAddr(req *http.Request) string {
	if req.URL != nil && req.URL.Host != "" {
		return req.URL.Host
	}
	return req.Host
}

// dialZap performs one ZAP-HTTP exchange for req over transport t and adapts
// it to the http.RoundTripper contract. zap-proto/http v0.1.0 speaks fasthttp:
// the inbound *http.Request is translated to a *fasthttp.Request, the round
// trip runs via Transport.Do, and the filled *fasthttp.Response is translated
// back to the *http.Response the caller expects. Do fully buffers the response
// and returns its pooled connection before it returns, so there is no
// caller-visible body/connection lifecycle to manage — unlike the prior
// net/http RoundTrip, whose Body.Close released the connection.
//
// SECURITY / do-not-enable notes (INGRESS_ZAP_BACKENDS is inert by default):
//   - v0.1.0 dials PLAINTEXT TCP (net.DialTimeout). It is NOT PQ-secure and
//     NOT encrypted — only route trusted in-cluster backends through it until
//     the transport negotiates a PQ/TLS handshake.
//   - The transport enforces a dial timeout (10s) and response-read timeout
//     (30s) but exposes no request-WRITE deadline; a slow-reading peer can
//     stall the write. Add a write deadline upstream before exposing this to
//     untrusted callers.
func (z *zapBackendRoundTripper) dialZap(t *zaphttp.Transport, req *http.Request) (*http.Response, error) {
	// Honour caller cancellation/deadline before spending work on the dial.
	if err := req.Context().Err(); err != nil {
		return nil, err
	}

	freq := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(freq)
	if err := httpToFastRequest(req, freq, z.maxBody); err != nil {
		return nil, err
	}

	fresp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(fresp)
	if err := t.Do(freq, fresp); err != nil {
		return nil, err
	}

	// Copy everything out of fresp before the deferred release recycles it.
	return fastToHTTPResponse(fresp, req), nil
}

// httpToFastRequest populates freq from req: method, origin-form request
// target, protocol, headers, host, body, and any declared trailers. Host and
// Content-Length are frame-owned by the ZAP-HTTP codec (Host rides fasthttp's
// Host slot, Content-Length is derived from the body), so they need no special
// casing beyond SetHost / SetBody. The request body is fully read and closed —
// the ZAP-HTTP frame carries the whole body in one shot.
func httpToFastRequest(req *http.Request, freq *fasthttp.Request, maxBody int64) error {
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	freq.Header.SetMethod(method)

	target := req.RequestURI
	if target == "" && req.URL != nil {
		target = req.URL.RequestURI()
	}
	freq.SetRequestURI(target)

	if req.Proto != "" {
		freq.Header.SetProtocol(req.Proto)
	}
	if req.Host != "" {
		freq.Header.SetHost(req.Host)
	} else if req.URL != nil && req.URL.Host != "" {
		freq.Header.SetHost(req.URL.Host)
	}

	for name, values := range req.Header {
		for _, v := range values {
			freq.Header.Add(name, v)
		}
	}

	if req.Body != nil {
		// A non-positive cap means "unset" (e.g. a directly-constructed
		// transport) — fall back to the default rather than rejecting all
		// bodies.
		if maxBody <= 0 {
			maxBody = defaultMaxZapBodyBytes
		}
		// Cap the buffered body. The ZAP-HTTP frame carries the whole body in
		// one shot, so an unbounded io.ReadAll lets a single oversized request
		// OOM the ingress. Read one byte past the cap to detect (and reject) an
		// over-limit body instead of silently truncating it.
		body, err := io.ReadAll(io.LimitReader(req.Body, maxBody+1))
		_ = req.Body.Close()
		if err != nil {
			return err
		}
		if int64(len(body)) > maxBody {
			return fmt.Errorf("zap-backend: request body exceeds %d-byte cap", maxBody)
		}
		freq.SetBody(body)
	}

	// Declared request trailers ride the frame's trailer slot; AddTrailer
	// registers the name so the codec routes it there rather than into the
	// headers map.
	for name, values := range req.Trailer {
		_ = freq.Header.AddTrailer(name)
		for _, v := range values {
			freq.Header.Add(name, v)
		}
	}
	return nil
}

// fastToHTTPResponse builds the *http.Response the RoundTripper contract
// expects from the fasthttp response Transport.Do filled. Body and header
// bytes are copied out of fresp so the caller may return it to fasthttp's
// pool. Header/trailer separation mirrors the ZAP-HTTP codec and net/http's
// own Transport: Content-Length and the Trailer meta-header are frame-owned
// (Content-Length is surfaced via Response.ContentLength), and declared
// trailers are lifted into Response.Trailer instead of being duplicated into
// Response.Header.
func fastToHTTPResponse(fresp *fasthttp.Response, req *http.Request) *http.Response {
	status := fresp.StatusCode()
	if status == 0 {
		status = http.StatusOK
	}
	reason := string(fresp.Header.StatusMessage())
	if reason == "" {
		reason = http.StatusText(status)
	}
	proto := string(fresp.Header.Protocol())
	if proto == "" {
		proto = zapHTTPProto
	}

	// Canonicalized set of declared trailer names, so their values are lifted
	// into Response.Trailer and excluded from Response.Header.
	var trailer http.Header
	trailerSet := map[string]bool{}
	if keys := fresp.Header.PeekTrailerKeys(); len(keys) > 0 {
		trailer = make(http.Header, len(keys))
		for _, k := range keys {
			ck := textproto.CanonicalMIMEHeaderKey(string(k))
			trailerSet[ck] = true
			for _, v := range fresp.Header.PeekAll(string(k)) {
				trailer[ck] = append(trailer[ck], string(v))
			}
		}
	}

	header := make(http.Header)
	fresp.Header.VisitAll(func(key, value []byte) {
		k := textproto.CanonicalMIMEHeaderKey(string(key))
		if k == "Content-Length" || k == "Trailer" || trailerSet[k] {
			return // frame-owned or lifted into Trailer
		}
		header[k] = append(header[k], string(value))
	})

	// The body is fully buffered inside fresp; copy it so the response holds
	// no reference into pooled fasthttp memory.
	body := append([]byte(nil), fresp.Body()...)

	return &http.Response{
		Status:        strconv.Itoa(status) + " " + reason,
		StatusCode:    status,
		Proto:         proto,
		ProtoMajor:    1,
		ProtoMinor:    0,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Trailer:       trailer,
		Request:       req,
	}
}
