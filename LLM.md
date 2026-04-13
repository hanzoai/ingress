# LLM.md - Hanzo Ingress

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

## Rebrand Notes (v3.8.0)

### External library constraints
- `github.com/traefik/paerser` defines `DefaultRootName = "traefik"` and `DefaultNamePrefix = "TRAEFIK_"`.
  Config file flags (`-traefik.configfile`), env var prefix (`TRAEFIK_*`), and parser root name
  **must** stay as `traefik` to match the external library. These are NOT candidates for rebrand.
- `github.com/traefik/yaegi`, `github.com/traefik/grpc-web`, `github.com/traefik/oxy` are external
  imports -- never touch their paths.

### Hash-bound test data
- `pkg/middlewares/auth/digest_auth_test.go` contains htdigest hashes computed as
  `md5(user:realm:password)` where realm=`"traefik"`. Changing the realm invalidates the hash.
  These 10 references must stay.

### Wire protocol changes (intentional)
- `X-Traefik-Fast-Proxy` header renamed to `X-Ingress-Fast-Proxy`
- `X-Traefik-Router` header renamed to `X-Ingress-Router`
- Prometheus/InfluxDB metric prefix: `traefik.*` renamed to `ingress.*`

### Pre-existing test failures (not caused by rebrand)
- `pkg/muxer/http/Test_addRoute/Host_IPv6`: Go 1.26 broke IPv6 URL parsing
- `pkg/middlewares/ratelimiter`: Timing-sensitive tests, flaky

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
