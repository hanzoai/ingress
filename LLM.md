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
- **Generated CRD manifests** — `kubernetes-crd-definition-v1.yml` and
  `integration/fixtures/k8s/01-ingress-crd.yml` are controller-gen output
  from `hanzoai/v1alpha1`, whose `+groupName` marker is `hanzo.ai`, so
  `script/code-gen.sh`'s `hanzo.ai_*.yaml` glob matches and the concat step
  runs. The stale `traefik.io_*.yaml` dumps are deleted. Regenerate the
  schema alone with the `controller-gen` line from that script; the
  clientset half still churns headers (below).
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
- `pkg/provider/kubernetes/crd` tests HANG (`TestClientIgnoresHelmOwnedSecrets`
  first, then every loader test) until the package timeout, on untouched main —
  measured in a clean worktree. The 132 fixtures now declare `hanzo.ai/v1alpha1`,
  which register.go registers; what still names the old group is the generated
  fake clientset (`generated/clientset/versioned/typed/hanzoai/v1alpha1/fake/
  fake_traefikio_client.go`), so the informer waits on a GVR the fake tracker
  never serves. The fix is `script/code-gen.sh`'s gen_client step, which also
  carries the header churn above; land the two together. Not in `hanzo.yml`'s
  test list, so it stops no build.
- Codegen header drift: `script/code-gen.sh` rewrites 118 generated files
  with the header from `boilerplate.go.tmpl` (`2020-2025 Traefik Labs;
  2025-2026 Hanzo AI Inc`) where the committed files read `2020-2026 Hanzo
  AI Inc`. The template is the correct one — MIT keeps the upstream notice —
  so the committed headers want restoring in a commit of their own; until
  then a wholesale run is noise. Its gen_helpers pass also truncates
  `kubernetes-crd-definition-v1.yml` to empty if the controller-gen step is
  skipped, so run the steps by hand, in order.

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

## WAF (`waf` middleware)

`pkg/middlewares/waf` is OWASP Coraza (`corazawaf/coraza/v3`) with the OWASP
Core Rule Set v4 embedded (`coraza-coreruleset/v4`, an `fs.FS`), reached from
every provider — file, labels, and the `Middleware` CR — through `dynamic.WAF`.
Rules load Core Rule Set → `directivesFiles` → `directives`; a configuration
that loads none is refused at build.

**The engine state is written after every rule, and that order is the
middleware.** `@coraza.conf-recommended`, the first file the Core Rule Set
loads, is `SecRuleEngine DetectionOnly` on line 7. Written first, our
`SecRuleEngine On` was overridden by it and the middleware loaded all of CRS
and refused nothing — every load-and-serve test green. `detectionOnly` is the
one place the mode is decided; `TestARulesetCannotDisarmTheFirewall` holds it,
mutation-checked by moving the directive back ahead of the rules (four tests
go red).

**A middleware reaches the CRD plane through three files, and `geoBlock` was
in none of them.** `dynamic.Middleware` (the field), `v1alpha1.MiddlewareSpec`
(the CR field), and both `zz_generated.deepcopy.go` (or the field is copied
out of every object the informer hands over), plus the controller-gen schema
or the apiserver prunes the key. `geoBlock` had the first only, so it worked
from the file provider and was silently absent from every `Middleware` CR —
a sanctions control that read as configured. All three carry `geoBlock` and
`waf` now; `TestEveryMiddlewareFieldSurvivesADeepCopy` is the gate, and it
fails when either copy is removed.

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

## Edge secrets — KMS, and the ACME store is sealed

The edge holds two things worth more than any request it proxies: the ACME
account key, which re-issues certificates for every domain the estate serves,
and the DNS credential the ACME challenge is answered with. Both used to be a
base64 field in a Kubernetes Secret, mounted into the process environment or
written to a node.

`pkg/kms` is where they come from now. It speaks the same luxfi/kms HTTP
contract the gateway's routes loader does — `POST /v1/kms/auth/login`, then
`GET /v1/kms/secrets/{path}/{name}?env=` — so the estate has one KMS
conversation, not two. The org is the token's, never a URL segment.

```
INGRESS_KMS_ENDPOINT      https://kms.hanzo.ai   (unset = no KMS; see below)
INGRESS_KMS_CLIENT_ID     ← IAM_CLIENT_ID
INGRESS_KMS_CLIENT_SECRET ← IAM_CLIENT_SECRET
INGRESS_KMS_ORG           default "hanzo"   (the lux overlay sets "lux")
INGRESS_KMS_ENV           default "default"
INGRESS_KMS_PATH          default "ingress"
INGRESS_ACME_ADOPT        unset; one boot, see "The seal"
```

The endpoint must be https — the client secret is in the login body and the
bearer is in a header of every read.

Which secrets it reads is a property of the service, not of the environment, so
the names are constants: `ingress/acme-seal` (32 bytes, hex or base64) and
`ingress/cloudflare-token`. Only the first is required; see the pre-flight at
the end of this section for which of them to create.

### The seal

`acme.Seal` (`pkg/provider/acme/seal.go`) is a two-method interface — `Wrap`,
`Unwrap` — and both persistent stores route their one serialize point through it
(`encodeStored` / `decodeStored`). The implementation is `kms.Seal`: envelope,
AES-256-GCM both ways. Each write mints a fresh 256-bit data key, encrypts the
document under it, and encrypts that key under the key from KMS. The long-lived
key therefore encrypts 48 bytes per write rather than the whole document, and
recovering one write's data key opens that write only.

A store is NAMED and every write is COUNTED, and both travel into the seal:

```
{"seal":1,"id":"<key fingerprint>","count":<n>,"key":"…","data":"…"}
```

`{version, store name, key id, count}` is the additional data of BOTH AEAD
layers, length-prefixed so no two field combinations render the same bytes. Each
of those four is therefore verified by the decryption rather than trusted from
the JSON. A document opens for the store it was written for (`/data/acme.json`
for the file store, `<namespace>/<secret>` for the shared one), under the key it
was written with, at the write it was made at:

- an envelope moved between stores does not open — `TestSeal_RefusesADocumentFromAnotherStore`
- an envelope edited to claim another count does not open — `TestSeal_CountCannotBeEdited`
- an envelope sealed under a retired key names that key — `TestSeal_NamesTheKeyItWasSealedUnder`

The count only moves forward (`counter`). A shared store re-reads its object on
every poll, so a replica that has read write 9 keeps write 9 rather than
stepping back to an earlier copy of the same document
(`TestSharedStore_RefusesAnEarlierDocument`). Counting starts at one; zero is
the count of a document never written under seal, and `Wrap` refuses it.

A store that is not sealed is REFUSED. `INGRESS_ACME_ADOPT` (any non-empty
value, unset everywhere by default) is the operator saying otherwise: it opens
one unsealed store, keeps its certificates rather than re-ordering every one of
them against a Let's Encrypt rate limit, and writes it back under seal.

Adoption ends the moment a sealed write lands (`Seal.Persisted`, called by both
stores after the write reaches disk or the Secret). After that the store is
sealed and reads open regardless; a store handed to the process afterwards that
is STILL unsealed is a different store — a pre-seal snapshot, replanted — and is
refused, even though it is the very bytes adoption admitted a moment ago. The
flag is spent within the process, not merely once per boot. It still warns on
every boot it is set, so it is not left on quietly, and a restart with the flag
still set re-adopts whatever plaintext is on disk then — which is why it is set
for one boot and removed. Adoption is a property of the READ; every write an
adopting seal makes is sealed, and the refusing seal over the same key reads it.

The envelope is FORWARD-ONLY. There is no compatibility path for an earlier
shape because there is no deployed sealed state — the `data` volume is
node-local ephemeral storage, backed by neither a Secret nor a ConfigMap.

`acme.Plain()` is the identity seal and is byte-for-byte what every deployment
wrote before this existed (`TestPlain_IsTheDocumentItself`). It is a null object
rather than a nil check, so no store has to ask whether it has a seal — that is
the check one of them would eventually forget, on the path whose whole job is to
not write private keys in the clear. It reports its document sealed, because for
that deployment the document IS the storage format and there is nothing to adopt.

A read that does not open is an ERROR line and a
`ingress_acme_unseal_failures_total{store}` count. The local store publishes its
state only once the file has been read, so a read that refuses stays refused —
publishing an empty map first left the next call looking at a store that
appeared new, and an ACME provider answers a new store by ordering the estate
again over what is already there (`TestLocalStore_RefusesAnUnsealedStore`).

Failure polarity, decided once in `cmd/ingress/secrets.go`:

- KMS configured, sealing key unreadable **within the bound** → **fatal**. Never
  a quiet downgrade to writing private keys in the clear; that failure happens
  when nobody is watching and leaves the account key readable on the node
  forever after.
- KMS configured, DNS token unreadable within the bound → **warn, keep
  serving**. DNS-01 is one challenge of three, an edge holding its certificates
  keeps serving TLS without ever calling Cloudflare, and the deployment may
  supply that credential directly.
- KMS not configured → `Plain()`, with a warning naming the file. An unsealed
  edge is a thing someone can see in the logs.
- Endpoint set but credentials empty, or endpoint not https → **fatal, not
  retried**. A configuration error does not heal.
- A seal failure at write time writes **nothing**.

Every KMS read at startup goes through `kms.Retry(reach, …)`. One deadline,
taken once, covers the attempt in flight as well as the waits between attempts,
and the wait is trimmed to what is left of the budget. `reach` is 30s — under
the window a liveness probe allows a starting container, because a retry the
kubelet outlives is not a retry.

### Cloudflare: the credential is swapped, never dropped

lego resolves DNS-provider credentials from the environment and offers no way to
hand a provider its credential directly, so the value passes through this
process's env either way. What is ours to decide is WHICH value.

`loadDNSCredential` is a SWAP and only a swap: it sets
`CLOUDFLARE_DNS_API_TOKEN` from KMS and, having done so, removes every other way
that credential could arrive. The removal is load-bearing rather than tidiness —
lego tries the account-global pair FIRST (`cloudflare.go` `NewDNSProvider`) and
only falls back to a token, so a pair left in the env means the token is never
read. Both spellings go: lego resolves each name through `env.GetOrFile`, so
`CF_API_KEY` and `CF_API_KEY_FILE` are the same credential and a set that covers
one covers neither.

When KMS holds no token, or cannot be reached within the bound, the environment
is left exactly as the manifest filled it. Removing the pair without a token to
put in its place leaves the challenge with no credential at all, on every node
at once (`TestLoadDNSCredential_UnreachableKMSLeavesTheEnvironment`).

Today both clusters supply Cloudflare through the `cloudflare-api-credentials`
Secret and there is no `ingress/cloudflare-token` in KMS. Moving to a
zone-scoped token is a SEPARATE change with its own window: it swaps the
mechanism lego authenticates with, and the pair and a token do not coexist.

The hostPath is unchanged and still node-local — sealing makes the file inert,
it does not make the store shared. `ACME_SHARED_STORE_NAMESPACE` is the fix for
one-order-per-node, and the seal applies there too.

## Deploy pre-flight — what a human creates BEFORE applying

The image and the manifest in this repo are safe to build at any time. Applying
them is not, until the sealing key and the credential that reads it exist.
Neither is created by the manifest and neither is created by the process.

**The sealing key, in KMS, per org that runs an edge.** One new secret, 32
bytes, hex or standard base64:

```
lux    kms.lux.cloud   ingress/acme-seal
hanzo  kms.hanzo.ai    ingress/acme-seal
```

Do NOT create `ingress/cloudflare-token` as part of this change — that is the
separate token migration described above, and creating it here gives the same
component the same credential two ways.

**The credential that reads it, in each cluster, in the workload's own
namespace.** Secret `ingress-kms` with keys `clientId` and `clientSecret` — the
IAM application this edge authenticates to KMS as (`lux-ingress`, `hanzo-ingress`; `client_credentials`
must be in its `grant_types`). Materialise it with a **KMSSecret**, which is the
mechanism already in use in these clusters, not by hand and not from a file.

The reference is deliberately NOT optional. This pod is configured for KMS, so
an absent Secret is an absent configuration, and a kubelet
`CreateContainerConfigError` naming `ingress-kms` says that more precisely than
a process that starts and then reports it cannot read anything.

**What applying without them does, per workload.** The two orgs run different
kinds and they fail differently:

| | kind | rollout | a pod that cannot start |
|---|---|---|---|
| lux | DaemonSet | `maxUnavailable: 1`, `maxSurge: 0` | the old pod on that node is deleted first, so that ONE node has no edge until a human rolls back; the rollout stalls there and the other nodes keep serving |
| hanzo | Deployment | `maxSurge: 1`, `maxUnavailable: 0` | the new pod must be Ready before an old one goes, so it never is, the rollout stalls, and both old pods keep serving |

Neither is a fleet-wide outage. The DaemonSet one is a real sustained
single-node outage and is the reason to check first.

**Only lux has a workload manifest here.** `k8s/lux/deployment.yaml` matches
what runs in `lux-system` (DaemonSet `hanzo-ingress`). Hanzo's workload is
declared in `hanzoai/universe` (`infra/k8s/ingress/`, Deployment `ingress`) and
`k8s/hanzo/` carries only the cluster-level objects it uses — RBAC, the
IngressClass, the `security-headers` Middleware.

The Deployment and Service that used to be in `k8s/hanzo/` described a
different workload from the one that runs: a `hostNetwork` DaemonSet named
`hanzo-ingress` binding `hostPort` 80 and 443. `kubectl apply -f k8s/hanzo/` is
in this repo's README and in the sibling gateway's Makefile, so that file was
one command away from taking :80 and :443 on every node in the cluster, beside
the Deployment that already had them, each pod with its own node-local ACME
store ordering for the same domains. It is deleted rather than corrected:
universe is the one place that says what runs, and a second copy here is a
second answer.

**Adoption.** A first boot against an empty `data` volume needs nothing: the
store is written sealed from its first write. `INGRESS_ACME_ADOPT` is only for a
node that already holds an unsealed `acme.json` worth keeping. It admits one
store, it WRITES (not a read-only inspection), and it is spent the moment that
store is sealed — a replanted pre-seal snapshot is refused thereafter. A restart
with the flag STILL set re-adopts whatever plaintext is on disk then, so set it
for one boot and remove it after.

**Rolling the image before any of this is done is safe, and does nothing.**
Universe declares no `INGRESS_KMS_*` for the hanzo edge and no `ingress-kms`
Secret, so `FromEnv` returns no client, the seal is `Plain()`, and the process
warns once and serves exactly as it does today. Nothing is sealed until universe
adds the env — which is the change that needs the pre-flight above, not the
image.
