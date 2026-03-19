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
