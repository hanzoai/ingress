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

The engine is fully de-branded: the parser dependency was forked to
`github.com/hanzoai/ingress-parser` (`DefaultRootName = "ingress"`,
`DefaultNamePrefix = "INGRESS_"`), the config base paths are
`/etc/ingress/ingress`, the Prometheus metric prefix is `ingress_`, the
CRD apiGroup is `hanzo.ai`, the internal entry point is `ingress`, and the
auth middlewares' default realm is `ingress`. The standalone binary is
built from `cmd/ingress` and ships as `hanzo-ingress`.

### Framework ownership — every fork owns its module path (#29)

The engine itself is a full source-fork under `github.com/hanzoai/ingress`
(own module path, whole tree). The auxiliary libraries it links follow the
same rule: **a hanzoai fork declares its OWN module path and is required
directly — never resolved via `replace` against the upstream path.** A
`replace` leaves the upstream brand on every import line in every consumer,
which is exactly what the rule exists to prevent.

| Import path | Version | Ownership | Used by |
|-------------|---------|-----------|---------|
| `github.com/hanzoai/yaegi` | v0.16.2 | fork (ours), own module path | `pkg/plugins` (Go interpreter for plugins) |
| `github.com/hanzoai/grpc-web` | v0.16.1 | fork (ours), own module path | `pkg/middlewares/grpcweb` |
| `github.com/hanzoai/ingress-parser` | v0.2.3 | fork (ours), own module path | config parser (`DefaultRootName=ingress`, `INGRESS_` prefix) |
| `github.com/vulcand/oxy/v2` | pseudo-v2.0.0-2026… | fork (ours), **still via `replace`** | reverse-proxy lib — `utils`/`forward`/`buffer`/`cbreaker`/`connlimit` across `pkg/middlewares/*` |

`go list -m all | grep hanzoai` shows yaegi/grpc-web/ingress-parser resolving
with no `replace` in play. oxy is the one remaining `replace`: it carries no
upstream brand in its path, so it is not a leak, but it does violate the
own-your-module-path rule. Closing it is a one-line change here once
`github.com/hanzoai/oxy` declares `module github.com/hanzoai/oxy/v2`.

### Load-bearing "traefik" that intentionally STAYS
Not branding — renaming these breaks behaviour or breaches a licence:

- **Licence attribution (legal)** — `LICENSE.md`, `NOTICE`, and the
  `script/boilerplate.go.tmpl` copyright header carry the upstream
  copyright line. Retaining it is an obligation of the MIT/Apache-2.0
  grant we forked under, not a branding choice. Never strip it.
- **k3s CLI flag** — `integration/resources/compose/k8s.yml` passes
  `--disable=traefik` to k3s, which is how k3s is told not to install its
  own bundled ingress. The token names *k3s's* component, not ours;
  renaming it silently re-enables that ingress and the test starts racing
  a second controller for port 80.
- **Upstream test images** — `integration/**` pulls `traefik/whoami`,
  `traefik/whoamitcp`, `traefik/whoamiudp`. These are published images on
  Docker Hub; the name is an address, not a label. Closing this needs the
  three images mirrored to `ghcr.io/hanzoai/whoami{,tcp,udp}` first.
- **Upstream issue citation** — `integration/consul_test.go` links
  `https://github.com/traefik/traefik/issues/8092` as the provenance of
  a regression test. The issue only exists at that URL; a rewritten link
  would be a dead link, which is worse than the mention.

### Known remaining leaks (tracked, not yet closed)
- **Generated CRD manifests** — `docs/content/reference/dynamic-configuration/`
  still holds `traefik.io_*.yaml`: stale controller-gen output whose
  *filenames and `description:` text* carry the brand, and which
  `kubectl explain` surfaces to customers. The Go CRD types they are
  generated from are already clean (`// IngressRoute is the CRD
  implementation of a Ingress HTTP Router.`), so this is pure staleness.
  Note `script/code-gen.sh` globs `hanzo.ai_*.yaml`, which matches nothing
  — the regeneration pipeline is broken and must be fixed before the
  dumps can be refreshed. `integration/fixtures/k8s/01-ingress-crd.yml`
  (the copy that is actually applied) has been hand-corrected to match
  what a fixed regeneration would emit.
- **Test certificate DN** — `integration/fixtures/acme/ssl/wildcard.crt`
  has `OU=Traefik` in its X.509 subject, documented by
  `integration/fixtures/acme/README.md`. Closing it means regenerating
  the cert/key pair with `OU=Ingress` and can only be validated by the
  acme integration suite (Docker + pebble).

### Vendored upstream content (out of scope by nature)
`docs/content/**` (prose), `CHANGELOG.md`, and `webui/**` (the dashboard
SPA) are imported upstream material kept for reference. They carry the bulk
of the remaining occurrences and do not ship in the runtime binary or image.
Rebranding them makes future upstream syncs harder; do it only as a
dedicated docs pass.

### Wire protocol (diverges from upstream — intentional)
- Internal proxy headers are `X-Ingress-Fast-Proxy` and `X-Ingress-Router`.
- Prometheus/InfluxDB metrics are prefixed `ingress.*` (the bundled Grafana
  dashboards in `contrib/grafana/ingress*.json` query `ingress_*`
  accordingly).
- The internal entry point is named `ingress`, and the basic/digest auth
  middlewares default to realm `ingress`.

Each of these differs from the name upstream uses, so a config, dashboard
or scrape rule written against upstream will silently no-op here. That is
the intended behaviour: there is one name for each of these, and it is ours.

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

## In-process surface (`App`, root package)

`app.go` builds the small surface a co-resident binary needs — `GET
/_/ingress/healthz` and a read-only `GET /_/ingress/config` (brand, domain,
expected entrypoints/providers, ACME on/off, from `INGRESS_*` env). The proxy
itself is unaffected: it is `cmd/ingress`, out of process, at the cluster edge.

```go
func App(brand, domain string) (*zip.App, error)
```

Two strings, because those are the only two facts it reports back. It took
`cloud.Deps` and registered itself into `cloud.Registry`, which made ingress
import its own host — and `hanzoai/cloud` ships two editions under one module
path, so `cloud.Deps` is a different type in each and only one of them could
ever compose this package. Returning the app instead of writing into one handed
in means a host holds what it gets back and mounts it where it likes; nothing
here knows a registry exists. Rule: a subsystem imports `zip`, never its host.

It was also behind a `cloud` build tag, so nothing compiled it and it had
drifted to symbols cloud no longer has. No tag now, and `.` is in `test-unit`
and in the `hanzo.yml` test list.

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

The K8s Ingress provider reads exactly one annotation prefix:
`ingress.kubernetes.io/`. Any other vendor prefix is silently ignored, so a
manifest carrying an upstream-prefixed annotation applies no configuration
and reports no error — check the prefix first when an annotation appears to
have no effect.

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
