```yaml tab="Docker & Swarm"
# Dynamic Configuration
labels:
  - "ingress.http.routers.dashboard.rule=Host(`ingress.example.com`) && PathPrefix(`/ingress`)"
  - "ingress.http.routers.dashboard.service=api@internal"
  - "ingress.http.routers.dashboard.middlewares=auth"
  - "ingress.http.middlewares.auth.basicauth.users=test:$$apr1$$H6uskkkW$$IgXLP6ewTrSuBkTrqE8wj/,test2:$$apr1$$d9hr9HBB$$4HxwgUir3HP4EsggP/QNo0"
```

```yaml tab="Docker (Swarm)"
# Dynamic Configuration
deploy:
  labels:
    - "ingress.http.routers.dashboard.rule=Host(`ingress.example.com`) && PathPrefix(`/ingress`)"
    - "ingress.http.routers.dashboard.service=api@internal"
    - "ingress.http.routers.dashboard.middlewares=auth"
    - "ingress.http.middlewares.auth.basicauth.users=test:$$apr1$$H6uskkkW$$IgXLP6ewTrSuBkTrqE8wj/,test2:$$apr1$$d9hr9HBB$$4HxwgUir3HP4EsggP/QNo0"
    # Dummy service for Swarm port detection. The port can be any valid integer value.
    - "ingress.http.services.dummy-svc.loadbalancer.server.port=9999"
```

```yaml tab="Kubernetes CRD"
apiVersion: hanzo.ai/v1alpha1
kind: IngressRoute
metadata:
  name: ingress-dashboard
spec:
  routes:
  - match: Host(`ingress.example.com`) && PathPrefix(`/ingress`)
    kind: Rule
    services:
    - name: api@internal
      kind: IngressService
    middlewares:
      - name: auth
---
apiVersion: hanzo.ai/v1alpha1
kind: Middleware
metadata:
  name: auth
spec:
  basicAuth:
    secret: secretName # Kubernetes secret named "secretName"
```

```yaml tab="Consul Catalog"
# Dynamic Configuration
- "ingress.http.routers.dashboard.rule=Host(`ingress.example.com`) && PathPrefix(`/ingress`)"
- "ingress.http.routers.dashboard.service=api@internal"
- "ingress.http.routers.dashboard.middlewares=auth"
- "ingress.http.middlewares.auth.basicauth.users=test:$$apr1$$H6uskkkW$$IgXLP6ewTrSuBkTrqE8wj/,test2:$$apr1$$d9hr9HBB$$4HxwgUir3HP4EsggP/QNo0"
```

```yaml tab="File (YAML)"
# Dynamic Configuration
http:
  routers:
    dashboard:
      rule: Host(`ingress.example.com`) && PathPrefix(`/ingress`)
      service: api@internal
      middlewares:
        - auth
  middlewares:
    auth:
      basicAuth:
        users:
          - "test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/"
          - "test2:$apr1$d9hr9HBB$4HxwgUir3HP4EsggP/QNo0"
```

```toml tab="File (TOML)"
# Dynamic Configuration
[http.routers.my-api]
  rule = "Host(`ingress.example.com`) && PathPrefix(`/ingress`)"
  service = "api@internal"
  middlewares = ["auth"]

[http.middlewares.auth.basicAuth]
  users = [
    "test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/",
    "test2:$apr1$d9hr9HBB$4HxwgUir3HP4EsggP/QNo0",
  ]
```
