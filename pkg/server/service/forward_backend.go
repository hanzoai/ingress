// Package service: forward_backend wires the canonical HIP-0110
// "HTTP-over-ZAP" Forward contract (github.com/luxfi/zap/forward) for
// selected AI routes. When a decrypted request's path matches the
// configured prefix allowlist, the request is packaged into a Forward
// envelope and Call'd to the gateway-relay's ZAP node instead of being
// dialed over net/http. Every other route falls through to the wrapped
// RoundTripper unchanged.
//
// This is the sibling of zap_backend.go. They are DIFFERENT protocols:
//
//   - zap_backend.go routes by upstream host:port over
//     github.com/zap-proto/http (zaphttp) — a transparent HTTP transport
//     swap to a zaphttp.Server backend.
//   - forward_backend.go routes by request-path prefix over
//     github.com/luxfi/zap/forward — it packages the request into the
//     Forward ENVELOPE that the gateway-relay (forward.Relay) authenticates
//     and forwards. The gateway-relay speaks the luxfi/zap Forward protocol,
//     not zaphttp, so this needs forward.Forwarder, not zaphttp.Transport.
//
// TLS termination is unaffected: Traefik still terminates the client TLS at
// the edge. This only changes what happens to the DECRYPTED request for the
// matched AI routes — it leaves the edge as a Forward envelope to the
// gateway-relay over ZAP.
//
// Identity discipline (delegated to forward.Forwarder.RoundTrip): the
// Authorization Bearer JWT is PRESERVED across the hop (the gateway-relay
// validates it), while every client-supplied identity header — X-Org-Id,
// X-User-Id, X-Roles, X-IAM-*, X-Hanzo-*, X-Gateway-*, identity cookies — is
// stripped before the envelope is built (forward/identity.go
// stripClientIdentity). A client cannot smuggle identity through this leg.
//
// Configuration (BOTH must be set to enable; either empty ⇒ pure
// pass-through, additive-only / opt-out by default):
//
//	INGRESS_FORWARD_ZAP_ADDR  — gateway-relay ZAP node addr, host:port, e.g.
//	                            gateway.hanzo.svc:9999
//	INGRESS_FORWARD_ROUTES    — comma-separated path-prefix allowlist, e.g.
//	                            /v1/chat,/v1/completions,/v1/models
package service

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	zaplib "github.com/luxfi/zap"
	"github.com/luxfi/zap/forward"

	"github.com/rs/zerolog/log"
)

const (
	// forwardZapAddrEnv names the gateway-relay's ZAP node address
	// (host:port). Empty disables the Forward leg entirely.
	forwardZapAddrEnv = "INGRESS_FORWARD_ZAP_ADDR"

	// forwardRoutesEnv names the comma-separated path-prefix allowlist for
	// AI routes that ride the Forward envelope. Empty disables the Forward
	// leg entirely (no route matches), so the address alone is inert.
	forwardRoutesEnv = "INGRESS_FORWARD_ROUTES"

	// forwardNodeID is this edge node's ZAP NodeID. The gateway-relay
	// resolves the caller by the NodeID it learns during the handshake;
	// a stable, descriptive id keeps relay logs legible.
	forwardNodeID = "ingress"

	// forwardNodePort is the local ZAP listen port for the edge node. The
	// edge never receives inbound Forward calls (it is purely a client of
	// the gateway-relay), but a zaplib.Node still binds a listener. 0 lets
	// the OS pick an ephemeral port so multiple edge processes never clash.
	forwardNodePort = 0
)

// forwardBackendRoundTripper packages matching requests into a Forward
// envelope and Call's the gateway-relay over ZAP; everything else is
// delegated to the wrapped RoundTripper. It holds a single process-wide
// forward.Forwarder (one ZAP node + one learned peer), created lazily on
// first matching request and reused.
type forwardBackendRoundTripper struct {
	next     http.RoundTripper
	gwAddr   string   // gateway-relay ZAP node addr (host:port)
	prefixes []string // path-prefix allowlist, longest-first irrelevant (HasPrefix)

	// nodes caches the singleton node+peer keyed by gwAddr (a sync.Map,
	// mirroring the gateway's getOrCreateNode caching idiom). The value is
	// a *forwardPeer once dialed; an in-flight dial is serialized by once.
	nodes sync.Map // gwAddr -> *forwardPeer
	once  sync.Map // gwAddr -> *sync.Once
}

// forwardPeer is the cached, dialed result: a started ZAP node connected to
// the gateway-relay, plus the forward.Forwarder bound to the learned peer.
// dialErr records a failed dial so a matching request fails closed (it does
// NOT silently fall through to net/http, which would bypass the gateway's
// auth/billing for an AI route).
type forwardPeer struct {
	node      *zaplib.Node
	forwarder *forward.Forwarder
	dialErr   error
}

// newZapEdgeRoundTripper is the single composition seam for ZAP transport
// decoration at the edge. It layers the two ORTHOGONAL, default-off ZAP legs
// over next, outermost first:
//
//  1. Forward leg (this file) — path-prefix routing of AI requests into the
//     HIP-0110 Forward envelope to the gateway-relay (luxfi/zap/forward).
//  2. ZAP-backend leg (zap_backend.go) — host:port routing of selected
//     backends over zaphttp (github.com/zap-proto/http).
//
// Each leg returns next unchanged when its env is empty, so with neither env
// set this is exactly next — additive, non-breaking. The Forward leg is
// outermost: it makes the higher-level (request-path) routing decision and,
// on no match, delegates DOWN through the ZAP-backend leg to the real
// transport. The two legs never both fire for one request (path vs host axes
// are independent), but the layering makes their precedence explicit.
//
// This is the one and only place these legs are composed; the transport
// call sites use this, never the per-leg constructors directly.
func newZapEdgeRoundTripper(next http.RoundTripper) http.RoundTripper {
	return newForwardBackendRoundTripper(newZapBackendRoundTripper(next))
}

// newForwardBackendRoundTripper wraps next. If either env var is empty it
// returns next unchanged so the call site stays oblivious — the Forward leg
// is opt-in and default-off (additive, non-breaking).
func newForwardBackendRoundTripper(next http.RoundTripper) http.RoundTripper {
	gwAddr := strings.TrimSpace(os.Getenv(forwardZapAddrEnv))
	prefixes := parseForwardRoutes(os.Getenv(forwardRoutesEnv))
	if gwAddr == "" || len(prefixes) == 0 {
		return next
	}
	log.Info().
		Str("gateway", gwAddr).
		Strs("routes", prefixes).
		Msg("[FORWARD-BACKEND] AI routes will tunnel to gateway-relay over ZAP Forward")
	return &forwardBackendRoundTripper{
		next:     next,
		gwAddr:   gwAddr,
		prefixes: prefixes,
	}
}

// parseForwardRoutes splits a comma-separated path-prefix list and trims
// whitespace. Empty entries are dropped. Each entry is matched with
// strings.HasPrefix against req.URL.Path.
func parseForwardRoutes(raw string) []string {
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

// matchPath reports whether req's path is in the prefix allowlist.
func (f *forwardBackendRoundTripper) matchPath(path string) bool {
	for _, p := range f.prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// RoundTrip routes a matching AI request over the Forward envelope to the
// gateway-relay; everything else delegates to the wrapped transport. A
// matching request whose ZAP dial failed fails closed (502) rather than
// silently downgrading to net/http and bypassing the gateway.
func (f *forwardBackendRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	path := "/"
	if req.URL != nil && req.URL.Path != "" {
		path = req.URL.Path
	}
	if !f.matchPath(path) {
		return f.next.RoundTrip(req)
	}

	peer := f.peer()
	if peer.dialErr != nil {
		// Fail closed: an AI route MUST traverse the gateway (auth +
		// billing). Do not fall through to net/http on a dial failure.
		log.Error().Err(peer.dialErr).Str("gateway", f.gwAddr).Str("path", path).
			Msg("[FORWARD-BACKEND] gateway-relay unreachable; failing closed")
		return nil, peer.dialErr
	}
	log.Debug().Str("gateway", f.gwAddr).Str("path", path).
		Msg("[FORWARD-BACKEND] tunneling request to gateway-relay over ZAP Forward")
	resp, err := peer.forwarder.RoundTrip(stripEdgeIdentity(req))
	if err != nil {
		// Self-heal: a Call error may mean the gateway-relay restarted and this
		// cached connection is dead. Evict so the NEXT matched request re-dials a
		// fresh node. This request still fails CLOSED (returns err — never falls
		// through to net/http and never bypasses gateway auth/billing).
		f.nodes.Delete(f.gwAddr)
		f.once.Delete(f.gwAddr)
	}
	return resp, err
}

// stripEdgeIdentity returns req with the four PROMOTABLE identity headers
// deleted, so the edge asserts NO identity into the Forward envelope.
//
// This is the load-bearing topology distinction. forward.Forwarder is the
// canonical GATEWAY-side client: it trusts X-Org-Id / X-User-Id /
// X-User-IsAdmin / X-User-Permissions and promotes them into the typed
// Forward identity fields, because by the time the gateway runs Forwarder it
// has ALREADY validated the JWT and re-set those headers itself. The ingress
// runs Forwarder one hop EARLIER — at the edge, UPSTREAM of JWT validation —
// where those same four headers are attacker-controlled. If we let them ride,
// requestToForward would promote a client-spoofed X-Org-Id into f.TenantID
// and the terminal would re-inject it as trusted identity.
//
// The gateway-relay (forward.Relay + its Gate) is what validates the
// Authorization Bearer JWT and injects the REAL identity. So the edge must
// assert none: delete the four promotable headers here. The remaining
// identity surface (X-Roles, X-User-Email, X-IAM-*, X-Hanzo-*, X-Gateway-*,
// Cookie) is scrubbed from the envelope's header blob inside
// Forwarder.RoundTrip's stripClientIdentity; only these four escape via the
// typed-field promotion, so deleting exactly them closes the leak completely.
// Authorization is deliberately left intact — it is the real auth the
// gateway-relay validates.
//
// The caller's *http.Request is NOT mutated: a shallow copy with a cloned,
// scrubbed Header is returned, since a RoundTripper must not modify the
// request it is given (Traefik may reuse/retry it).
func stripEdgeIdentity(req *http.Request) *http.Request {
	clone := req.Clone(req.Context())
	clone.Header.Del(forward.HeaderOrgID)       // X-Org-Id
	clone.Header.Del(forward.HeaderUserID)      // X-User-Id
	clone.Header.Del(forward.HeaderUserIsAdmin) // X-User-IsAdmin
	clone.Header.Del(forward.HeaderUserPerms)   // X-User-Permissions
	return clone
}

// peer returns the singleton dialed forward.Forwarder for the gateway-relay,
// creating it exactly once. Mirrors the gateway's getOrCreateNode double-
// check: a sync.Once per gwAddr serializes the dial; the result (success or
// dialErr) is cached in the nodes map and reused by every later request.
func (f *forwardBackendRoundTripper) peer() *forwardPeer {
	if v, ok := f.nodes.Load(f.gwAddr); ok {
		return v.(*forwardPeer)
	}
	onceVal, _ := f.once.LoadOrStore(f.gwAddr, &sync.Once{})
	once := onceVal.(*sync.Once)
	once.Do(func() {
		f.nodes.Store(f.gwAddr, f.dial())
	})
	v, _ := f.nodes.Load(f.gwAddr)
	return v.(*forwardPeer)
}

// dial starts the edge ZAP node and ConnectDirectID's the gateway-relay,
// learning its NodeID for the Forwarder.Peer. NoDiscovery is set: the edge
// reaches the gateway by its configured address, not via mDNS.
func (f *forwardBackendRoundTripper) dial() *forwardPeer {
	node := zaplib.NewNode(zaplib.NodeConfig{
		NodeID:      forwardNodeID,
		Port:        forwardNodePort,
		NoDiscovery: true,
		Logger:      slog.Default(),
	})
	if err := node.Start(); err != nil {
		return &forwardPeer{dialErr: err}
	}
	peerID, err := node.ConnectDirectID(f.gwAddr)
	if err != nil {
		node.Stop()
		return &forwardPeer{dialErr: err}
	}
	return &forwardPeer{
		node:      node,
		forwarder: &forward.Forwarder{Node: node, Peer: peerID},
	}
}
