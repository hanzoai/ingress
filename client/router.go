// Package client — canonical client interface for Hanzo Ingress.
//
//	import ing "github.com/hanzoai/ingress/client"
//	var r ing.Router = hanzoingress.NewRouter(cfg)

package client

import "context"

// Router is the L7 control surface: declare/update/list virtual hosts +
// route tables + middleware chains. The data plane (request forwarding)
// runs in-process inside the ingress binary; this interface is the
// management API every operator + admin tool uses.
type Router interface {
	// Kind reports the backend identifier
	// (hanzo-ingress | envoy | nginx).
	Kind() string

	// UpsertVHost creates or replaces a virtual host. Idempotent.
	UpsertVHost(ctx context.Context, v VHost) error

	// DeleteVHost removes a virtual host. Idempotent.
	DeleteVHost(ctx context.Context, name string) error

	// ListVHosts returns every virtual host in scope.
	ListVHosts(ctx context.Context) ([]VHost, error)

	// UpsertRoute creates or replaces a route on the named vhost.
	UpsertRoute(ctx context.Context, vhost string, r Route) error

	// DeleteRoute removes a route.
	DeleteRoute(ctx context.Context, vhost, routeID string) error

	// Reload triggers a hot-reload of the running data plane. Returns the
	// new config version id on success.
	Reload(ctx context.Context) (string, error)

	// Health reports the data-plane state.
	Health(ctx context.Context) (*Status, error)
}

// VHost is a virtual host. One vhost handles one ServerName + ports.
type VHost struct {
	// Name is the canonical identifier used in routes and reload logs.
	Name string
	// Hosts is the SNI / Host-header match set (apex + wildcards).
	Hosts []string
	// TLS configures HTTPS. Empty means HTTP-only.
	TLS *TLSConfig
	// HTTP3 enables QUIC + Alt-Svc advertisement.
	HTTP3 bool
	// EntryPoints names the listener IDs this vhost binds to
	// (web | websecure | grpc | internal).
	EntryPoints []string
	// Labels carry routing metadata (env, tenant, app) consumed by
	// dashboards and policy plugins.
	Labels map[string]string
}

// TLSConfig is the HTTPS settings for a vhost.
type TLSConfig struct {
	// SecretRef points at a KMS-managed cert/key pair (preferred) or a
	// k8s tls Secret (legacy). One of SecretRef or InlineCert+InlineKey
	// must be set.
	SecretRef    string
	InlineCert   []byte
	InlineKey    []byte
	// MinVersion: TLS1.2 | TLS1.3. Empty defaults to TLS1.3.
	MinVersion string
	// ALPN is the negotiated-protocol allowlist (h2 | http/1.1 | h3).
	ALPN []string
}

// Route is one L7 rule on a vhost.
type Route struct {
	ID         string
	Path       string // exact | prefix= | regex=
	Methods    []string
	Backend    Backend
	Middleware []Middleware
	// Priority disambiguates overlapping rules; higher wins.
	Priority int
}

// Backend is the upstream target.
type Backend struct {
	// Kind: k8s_service | static | function | redirect.
	Kind string
	// For Kind=k8s_service: Namespace + Name + Port.
	Namespace string
	Name      string
	Port      int
	// For Kind=static: explicit upstream URLs.
	URLs []string
	// For Kind=redirect: target URL + status.
	RedirectTo     string
	RedirectStatus int
}

// Middleware is one filter applied in the route's chain.
type Middleware struct {
	// Kind: rate_limit | auth_iam | auth_basic | cors | rewrite |
	// strip_prefix | redirect | request_size_limit | gzip | brotli |
	// otel_trace | plugin.
	Kind string
	// Config is the kind-specific config blob.
	Config map[string]any
}

// Status is the data-plane health snapshot.
type Status struct {
	Version       string // config version id
	UpSince       int64  // unix seconds
	VHostCount    int
	RouteCount    int
	ActiveConns   int
	Ready         bool
}
