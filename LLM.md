# Hanzo Ingress

## Overview
Go module: github.com/hanzoai/ingress/v3

## Tech Stack
- **Language**: Go

## Build & Run
```bash
go build ./...
go test ./...
```

## Structure
```
ingress/
  CHANGELOG.md
  CODE_OF_CONDUCT.md
  CONTRIBUTING.md
  Dockerfile
  Dockerfile.bin
  LICENSE.md
  LLM.md
  Makefile
  README.md
  SECURITY.md
  cmd/
  contrib/
  docs/
  generate.go
  go.mod
```

## Key Files
- `README.md` -- Project documentation
- `go.mod` -- Go module definition
- `Makefile` -- Build automation
- `Dockerfile` -- Container build

## Rebrand Notes

The engine is fully de-traefik'd: the parser dependency was forked to
`github.com/hanzoai/ingress-parser` (`DefaultRootName = "ingress"`,
`DefaultNamePrefix = "INGRESS_"`), the config base paths are
`/etc/ingress/ingress`, the Prometheus metric prefix is `ingress_`, and
the CRD apiGroup is `hanzo.ai`. The standalone binary is built from
`cmd/ingress` (was `cmd/traefik`) and ships as `hanzo-ingress`.

### Framework ownership — the `traefik/*` runtime libs (#29)

The Traefik engine itself is already a full source-fork under
`github.com/hanzoai/ingress` (own module path, whole tree). The auxiliary
upstream libraries the engine links are owned by pinning them to hanzoai
source-forks via `replace` in `go.mod` — the import lines stay
`github.com/traefik/*` (the forks keep the upstream module path at the same
version tag), only resolution moves to a path hanzoai controls.

| Upstream import | Resolves to | Ownership | Used by |
|-----------------|-------------|-----------|---------|
| `github.com/traefik/yaegi` v0.16.1 | **`github.com/hanzoai/yaegi` v0.16.1** | fork (ours) | `pkg/plugins` (Go interpreter for plugins) |
| `github.com/traefik/grpc-web` v0.16.0 | **`github.com/hanzoai/grpc-web` v0.16.0** | fork (ours) | `pkg/middlewares/grpcweb` |
| `github.com/vulcand/oxy/v2` | **`github.com/hanzoai/oxy/v2` v2.0.0-20260126093803-fb11d60e0fdf** | fork (ours) | reverse-proxy lib — `utils`/`forward`/`buffer`/`cbreaker`/`connlimit` across `pkg/middlewares/*` |
| `github.com/hanzoai/ingress-parser` | — | fork (ours) | config parser (`DefaultRootName=ingress`, `INGRESS_` prefix) |

`go list -m github.com/traefik/{yaegi,grpc-web} github.com/vulcand/oxy/v2` shows
all three `=> hanzoai/*`. `hanzoai/oxy` is a straight source-mirror of
`traefik/oxy` (itself a fork of `vulcand/oxy`) pinned to the exact commit
previously resolved — the module path stays `github.com/vulcand/oxy/v2`, so it
is a drop-in and the import lines are unchanged.

### Load-bearing "traefik" that intentionally STAYS
These are the only remaining `traefik` strings outside the vendored
upstream docs/changelog. They are NOT branding and must not be renamed:

- **External Go import paths** — the import lines `github.com/traefik/yaegi`,
  `github.com/traefik/grpc-web`, `github.com/vulcand/oxy/v2` stay verbatim; all
  three resolve to hanzoai forks via `replace` (see the ownership table above),
  so the path is upstream-looking but hanzoai-owned. Renaming the import path
  itself breaks the build.
- **Hash-bound test data** — `pkg/middlewares/auth/digest_auth_test.go`
  htdigest hashes are `md5(user:realm:password)` with realm `"traefik"`.
  Changing the realm invalidates the precomputed hashes (~10 refs).
- **Upstream issue citation** — `integration/consul_test.go` links
  `https://github.com/traefik/traefik/issues/8092` as the provenance of
  a regression test. The issue only exists at that URL.

### Vendored upstream content (out of scope by nature)
`docs/content/**`, `CHANGELOG.md`, `webui/**` (the dashboard SPA), and
the `traefik.io_*.yaml` CRD reference dumps under `docs/` are imported
upstream Traefik material kept for reference. They carry the bulk of the
remaining `traefik` occurrences and do not ship in the runtime binary or
image. Rebrand them only as part of a dedicated docs pass.

### Wire protocol changes (intentional)
- `X-Traefik-Fast-Proxy` header renamed to `X-Ingress-Fast-Proxy`
- `X-Traefik-Router` header renamed to `X-Ingress-Router`
- Prometheus/InfluxDB metric prefix: `traefik.*` renamed to `ingress.*`
  (the bundled Grafana dashboards in `contrib/grafana/ingress*.json`
  query the `ingress_*` metric names accordingly)

### Pre-existing build/test issues (not caused by rebrand)
- `webui/embed.go` `//go:embed static` fails until the dashboard assets
  are built (`make generate-webui`); `go build ./...` therefore needs the
  webui built first, or build the subset
  `go build $(go list ./... | grep -v /webui)`.
- `pkg/muxer/http/Test_addRoute/Host_IPv6`: Go 1.26 broke IPv6 URL parsing
- `pkg/middlewares/ratelimiter`: Timing-sensitive tests, flaky
- `pkg/provider/kubernetes/crd` tests panic: `no kind "IngressService" is
  registered for version "ingress.k8s.io/v1alpha1"` — the CRD Go types use
  apiGroup `hanzo.ai` (register.go) but several `kubernetes_test.go` fixtures
  still declare `apiVersion: ingress.k8s.io/v1alpha1`. Group-name drift in the
  test fixtures; the provider itself builds and runs on `hanzo.ai`.
- Codegen drift: `script/code-gen.sh` regenerates 130+ files with a copyright
  header from `boilerplate.go.tmpl` that differs from the committed headers,
  and `controller-gen` emits group `ingress.k8s.io` (stale `+groupName` marker
  in `hanzoai/v1alpha1/doc.go`) instead of `hanzo.ai`. Re-running it wholesale
  produces branding/group noise; hand-apply small generated-code changes and
  extract only the intended CRD-schema block from a scratch run.

## Static file serving (`staticFiles` middleware)
`pkg/middlewares/staticfiles` serves a site directly at the edge — the shared
static plane for the fleet (one ingress, unlimited sites). `Root` is a union:
- a local path (`/var/www`) serves from disk (`http.Dir`), or
- an `s3://bucket/prefix` URL serves from an object store (`s3.go`, over
  `hanzos3/go-sdk`). The object stream is the SDK's seekable object, so
  `http.ServeContent` streams and Ranges without buffering the whole bundle.

Both origins run through the same handler, so index resolution, `spaMode`
fallback, `errorPage404` and `cacheControl` behave identically. `spaMode` is
the only difference between an SPA site and a plain static site.

The object store is defined **only** by the ingress environment — one shared
store for the whole fleet: `S3_ENDPOINT`, `S3_REGION`, and credentials from
`AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`. A `Middleware` carries only `root`
(+`spaMode`/`spaIndex`/index/cache/404); it can neither point the ingress
credential at another host nor leak a secret into the dynamic-config plane. The
middleware build **fails closed** on an empty prefix (`s3://cdn` would expose
the whole bucket), a missing endpoint, or missing credentials.

Isolation and behavior:
- Keys are joined traversal-safe under the middleware's non-empty prefix; a
  request can never read another site's prefix.
- Under `spaMode`, a not-found *navigation* (extensionless path) returns the
  shell, but a missing *asset* (a non-HTML extension) returns 404 — a broken
  deploy stays visible, not masked by a 200 `index.html`.
- Errors map by cause: missing→404/SPA, access-denied→403, object-store
  outage→502 (availability is not authorization). Error responses are not cached.
- `index.html` (the shell) defaults to `Cache-Control: no-cache` (revalidated
  via ETag/Last-Modified); other assets keep the long default or a `cacheControl`
  override. Every served response carries `X-Content-Type-Options: nosniff`, and
  content-type is derived from the file extension (S3 metadata is not trusted).
- Object reads are bound to the request context, so a client disconnect cancels
  the upstream read (5-minute backstop remains).

`staticFiles` serves terminally (never calls `next`), so an attached route's
backend service is never reached — point it at an empty/placeholder service.
The `shadow`-tagged test drives the real object-store path against a live S3.

## Header Passthrough Behavior (2026-04-13)

The ingress controller correctly passes through ALL backend response headers.
Both the `httputil.ReverseProxy` path and the fast proxy path copy response
headers verbatim via `VisitAll` / `Header().Add()`.

The `security-headers` Middleware CRD (k8s/hanzo/middlewares.yaml) uses
`unrolled/secure` to ADD secure headers (HSTS, X-Frame-Options, etc.) via
`ModifyResponseHeaders`. It uses `res.Header.Set()`, which overwrites same-named
backend headers. This is intentional -- ingress-level security headers override
backend headers.

The `contenttype.DisableAutoDetection` wrapper on the entrypoint sets
`Content-Type` to nil in the header map before handler execution. This prevents
Go's default content-type sniffing but does not strip any other headers.

No code in the ingress adds `Content-Disposition`. If you see
`Content-Disposition: inline; filename="index.html"` in responses, it comes from
the backend (Go's `http.ServeContent` or `http.ServeFile`).

### Annotation prefix

The K8s Ingress provider uses annotation prefix `ingress.kubernetes.io/`, NOT
`traefik.ingress.kubernetes.io/`. Annotations with the old Traefik prefix are
silently ignored.

## ZAP-HTTP Backend Transport

`pkg/server/service/zap_backend.go` wraps every backend `RoundTripper`
returned by `TransportManager.createRoundTripper`. When the request's
upstream `host:port` (i.e. `req.URL.Host` at transport time) matches the
allowlist from env `INGRESS_ZAP_BACKENDS` (comma-separated), the request
is dialed via `github.com/zap-proto/http` instead of `net/http`. All
other backends fall through unchanged. Empty / unset env keeps the
wrapped transport untouched (no allocation, no overhead).

`zaphttp.NewTransport` pins to one peer (it ignores `req.URL.Host`
internally), so we keep one `*zaphttp.Transport` per backend address in
a `sync.Map`.

External (client-facing) TLS termination is untouched. CRD schema is
untouched. Reference doc:
`docs/content/reference/dynamic-configuration/zap-backend.md`.
