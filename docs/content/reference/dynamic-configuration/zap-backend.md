---
title: "ZAP-HTTP Backend Transport"
description: "Use github.com/zap-proto/http for the ingress→backend hop on selected services."
---

# ZAP-HTTP Backend Transport

The ingress controller can dial selected backend services using
[`github.com/zap-proto/http`](https://github.com/zap-proto/http) instead
of `net/http`. External TLS termination is unchanged; only the
in-cluster hop changes wire format.

## Enable

Set the `INGRESS_ZAP_BACKENDS` environment variable on the ingress pod
to a comma-separated list of `host:port` targets that should use
ZAP-HTTP. Any backend whose `host:port` is **not** in the list keeps
using the standard HTTP/HTTPS transport — the change is purely additive.

```yaml
env:
  - name: INGRESS_ZAP_BACKENDS
    value: "gateway.hanzo.svc.cluster.local:8080,iam.hanzo.svc.cluster.local:8000"
```

When `INGRESS_ZAP_BACKENDS` is unset or empty, the controller behaves
identically to a build without this feature.

## IngressRoute example

No CRD field changes. The IngressRoute simply points at the host:port
that appears in the allowlist; the controller swaps the transport based
on the backend address.

```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRoute
metadata:
  name: gateway
  namespace: hanzo
spec:
  entryPoints:
    - websecure
  routes:
    - match: Host(`api.hanzo.ai`)
      kind: Rule
      services:
        - name: gateway
          port: 8080
  tls:
    secretName: hanzo-ai-tls
```

If the resolved upstream for the `gateway` service is
`gateway.hanzo.svc.cluster.local:8080` and that address is in
`INGRESS_ZAP_BACKENDS`, the ingress will dial it via ZAP-HTTP.

## Notes

- The wire format is `github.com/zap-proto/http` v0.1: length-prefixed
  Cap'n Proto frames over TCP. v0.2 will add the X-Wing PQ KEM
  handshake.
- The backend service must speak ZAP-HTTP (use `zaphttp.ListenAndServe`
  or `zaphttp.Server`). A regular `net/http` server will not negotiate.
- HTTP/2, HTTP/3, and WebSocket are not supported on ZAP-routed
  backends; v0.2 of zap-http will add streaming and pooling.
- The lookup is by `host:port` of the resolved upstream (the value of
  `req.URL.Host` at transport time), not by the public-facing domain.
