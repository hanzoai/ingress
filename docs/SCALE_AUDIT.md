# ingress — horizontal-scale audit

Branch: `feat/edge-scale-cleanup`.

## Checklist

| Item | Status | Evidence / fix |
|---|---|---|
| No application logic. Routing only. | PASS | `mount.go` exposes only `/_/ingress/healthz` + `/_/ingress/config` (read-only view). The standalone binary (`cmd/traefik/`) is the upstream proxy — providers, TLS, ACME, no app logic. |
| No JWT validation | PASS | `grep -rn "JWT\|jwt\." mount.go doc.go` returns no auth refs. JWT validation lives in gateway (auth_middleware.go). |
| No identity-header minting | PASS | `grep -rn "X-Org-Id\|X-User-Id" *.go` (top-level) returns empty for the ingress-as-subsystem package. Identity headers are minted by gateway only — confirmed by HIP-0026 + the strip-list contract test in gateway's auth_middleware_security_test.go. |
| No body inspection | PASS | Ingress is L7 reverse-proxy at the TLS-termination boundary. Body is opaque bytes. No content-type-aware middleware in this repo (security-headers, redirect, rate-limit work at headers/URL level only). |
| Connection limits per upstream are sane | PASS | `RespondingTimeouts.ReadTimeout=60s`, `IdleTimeout=180s` (pkg/config/static/static_config.go:46-50). Per-route limit configurable via `inFlightReq` middleware CRD. Cluster pin keeps `maxConnsPerHost` at fasthttp defaults. The hostNetwork=true deployment caps per-node connections at OS-level (`net.core.somaxconn`). |
| Health endpoints exposed (no auth) | PASS | `/ping` on entrypoint:web (deployment.yaml:127). Liveness + readiness probes both hit `/ping` (deployment.yaml:185-198). `/_/ingress/healthz` from the cloud-mount is a second probe surface, also unauthenticated by design. |
| Reasonable timeouts | PASS | `ReadTimeout=60s`, `IdleTimeout=180s`. Configurable per entrypoint. Per-route `forwardingTimeouts` defaults to `dialTimeout=30s` + `responseHeaderTimeout=0s` (no cap, intentional for streaming). |
| Stateless. Replicas can come and go. No sticky sessions. | PASS | Hanzo ingress runs 4 replicas via Deployment + hostNetwork=true + podAntiAffinity (universe/infra/k8s/ingress/deployment.yaml:53-96). No session affinity. ACME state is on hostPath `/var/lib/hanzo-ingress` — per-node, not shared, and the k8s K8s resolver discovers certs from cert-manager Secrets, not from disk. ACME storage is the only "state" and it is per-replica regeneration on cold start (Let's Encrypt rate-limits this; we ride DNS-01 with Cloudflare so the rate-limit budget is comfortable). |
| `traefik` brand contamination | KNOWN | External library hard-coded names — `paerser.DefaultRootName="traefik"`, `paerser.DefaultNamePrefix="TRAEFIK_"`, htdigest test realm=`"traefik"`. Per LLM.md these are intentional and not candidates for rebrand. Wire-protocol headers were renamed: `X-Ingress-Fast-Proxy`, `X-Ingress-Router`. Prometheus prefix renamed `traefik.*`→`ingress.*`. The brand-leak is a forced consequence of the upstream parser library, not a defect. |

## What changed in this branch

Documentation only. The ingress mount surface was already correctly scoped before this audit: two read-only endpoints, zero application logic, zero JWT, zero identity. No source changes required.

## Things deliberately NOT done in this branch

- Universe manifests for ingress (HPA, PDB) — ingress is `hostNetwork: true` + 4-replica Deployment + podAntiAffinity. HPA on hostNetwork pods is incoherent (you cannot scale beyond worker-node count without scheduling collisions). PDB on a 4-pod set where each binds host ports is also incoherent (the loss of one pod equals the loss of one node IP from the Cloudflare-load-balanced RRset, so disruption budgets are a Cloudflare-side concern, not k8s). Leaving the deployment as-is is correct.
- Splitting ingress + gateway pods — already separate Deployments in `universe/infra/k8s/ingress/` and `universe/infra/k8s/gateway/`. The brief's task #3 is moot.
