# Hanzo Ingress

Cloud-native L7 reverse proxy and load balancer for Hanzo AI infrastructure. Kubernetes-native with automatic TLS, dynamic configuration, and zero-downtime reloads.

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/hanzoai/ingress/blob/main/LICENSE.md)

## Overview

Hanzo Ingress is the front door for all Hanzo production traffic. It watches Kubernetes Ingress resources, automatically provisions TLS certificates via Let's Encrypt, and routes traffic to internal services -- including [Hanzo Gateway](https://github.com/hanzoai/gateway) for API endpoints and direct service routing for web applications.

Deployed on the `hanzo-k8s` cluster as the default IngressClass (`hanzo`), it handles all `*.hanzo.ai` traffic with 2 replicas in host-network mode for direct port 80/443 binding.

## Features

- **Kubernetes-native** -- watches Ingress resources, auto-configures routes
- **Automatic TLS** -- Let's Encrypt certificate provisioning and renewal (wildcard support)
- **Dynamic configuration** -- zero-restart config updates as Ingress resources change
- **HTTP/2, gRPC, WebSocket** -- full protocol support for all backend types
- **Circuit breakers** -- automatic failure isolation with configurable thresholds
- **Retry logic** -- built-in retry with exponential backoff
- **Access logging** -- JSON and Common Log Format output
- **Metrics export** -- Prometheus, Datadog, StatsD, InfluxDB, OTLP
- **Web dashboard** -- built-in UI for route visualization and health monitoring
- **REST API** -- programmatic access to configuration and status
- **Single static binary** -- no runtime dependencies, minimal attack surface
- **Host-network mode** -- direct port binding for minimal latency

## Quick Start

### Build from Source

```bash
# Build the binary
make build

# Run with a config file
./hanzo-ingress --configFile=config.toml

# Run with CLI flags
./hanzo-ingress \
  --entrypoints.web.address=:80 \
  --entrypoints.websecure.address=:443 \
  --providers.kubernetesingress=true
```

### Docker

```bash
# Build the image
make docker

# Run
docker run -d \
  -p 80:80 \
  -p 443:443 \
  ghcr.io/hanzoai/ingress:latest \
  --entrypoints.web.address=:80 \
  --entrypoints.websecure.address=:443 \
  --ping=true
```

### Docker Compose

```yaml
services:
  ingress:
    image: ghcr.io/hanzoai/ingress:latest
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./acme.json:/acme.json
    command:
      - "--entrypoints.web.address=:80"
      - "--entrypoints.websecure.address=:443"
      - "--providers.docker=true"
      - "--certificatesresolvers.letsencrypt.acme.httpchallenge.entrypoint=web"
      - "--certificatesresolvers.letsencrypt.acme.email=ops@hanzo.ai"
      - "--certificatesresolvers.letsencrypt.acme.storage=/acme.json"
```

## Kubernetes Deployment

### IngressClass

Hanzo Ingress registers as the default IngressClass on the cluster:

```yaml
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: hanzo
  annotations:
    ingressclass.kubernetes.io/is-default-class: "true"
spec:
  controller: hanzo.ai/ingress-controller
```

Any Ingress resource without an explicit `ingressClassName` is automatically picked up.

### Deploy to hanzo-k8s

```bash
# Apply all manifests (RBAC, IngressClass, Deployment, Service)
kubectl --context do-sfo3-hanzo-k8s apply -f k8s/hanzo/

# Verify
kubectl --context do-sfo3-hanzo-k8s -n hanzo get pods -l app=hanzo-ingress
kubectl --context do-sfo3-hanzo-k8s -n hanzo get svc hanzo-ingress
```

### K8s Manifests

```
k8s/hanzo/
  rbac.yaml             # ServiceAccount, ClusterRole, ClusterRoleBinding
  ingressclass.yaml     # IngressClass "hanzo" (default)
  deployment.yaml       # 2 replicas, hostNetwork, ports 80/443
  service.yaml          # LoadBalancer service
```

### Production Configuration

| Property | Value |
|----------|-------|
| **Image** | `ghcr.io/hanzoai/ingress:latest` |
| **Replicas** | 2 |
| **Namespace** | `hanzo` |
| **Network** | hostNetwork (direct port binding) |
| **Ports** | 80 (HTTP), 443 (HTTPS) |
| **Service type** | LoadBalancer |
| **Health check** | `GET /ping` on port 80 |
| **Liveness probe** | HTTP `/ping`, 5s initial, 10s interval |
| **Readiness probe** | HTTP `/ping`, 3s initial, 5s interval |
| **Resources** | 100m-1000m CPU, 128Mi-512Mi memory |
| **Security** | `NET_BIND_SERVICE` capability, all others dropped |

### Creating Ingress Resources

Once deployed, create standard Kubernetes Ingress resources to route traffic:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-service
  namespace: hanzo
  annotations:
    # TLS via Let's Encrypt (if cert-manager or ACME is configured)
    cert-manager.io/cluster-issuer: letsencrypt
spec:
  ingressClassName: hanzo
  tls:
  - hosts:
    - my-service.hanzo.ai
    secretName: my-service-tls
  rules:
  - host: my-service.hanzo.ai
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: my-service
            port:
              number: 8080
```

## Architecture

```
                Internet
                   |
          +--------+--------+
          | Cloudflare CDN  |
          | (DNS + WAF)     |
          +--------+--------+
                   |
          +--------+--------+
          | Hanzo Ingress   |   <-- L7 reverse proxy
          | (ports 80/443)  |       TLS termination
          | IngressClass:   |       Route matching
          |   "hanzo"       |       Load balancing
          +--+----+----+----+
             |    |    |
    +--------+ +--+--+ +--------+
    | Gateway | | IAM | | Cloud  |   <-- Internal services
    | :8080   | |:8000| | :3000  |
    +---------+ +-----+ +--------+
```

### Request Flow

1. DNS resolves `*.hanzo.ai` to Cloudflare
2. Cloudflare proxies to hanzo-k8s cluster LB (`24.199.76.156`)
3. Hanzo Ingress terminates TLS, matches host/path rules
4. Request forwarded to the matching backend service
5. For API traffic (`api.hanzo.ai`), Ingress routes to Hanzo Gateway for endpoint-level routing

## Service Discovery

Hanzo Ingress supports multiple provider backends:

| Provider | Description |
|----------|-------------|
| **Kubernetes Ingress** | Watches `networking.k8s.io/v1` Ingress resources (primary) |
| **Docker** | Discovers containers via Docker socket labels |
| **File** | Static TOML/YAML configuration files |
| **Consul** | Service catalog integration |
| **Etcd** | Key-value store configuration |
| **ECS** | AWS ECS task discovery |

Production runs exclusively with the Kubernetes Ingress provider.

## Configuration

### CLI Flags (Production)

```bash
./hanzo-ingress \
  --providers.kubernetesingress=true \
  --providers.kubernetesingress.ingressendpoint.publishedservice=hanzo/hanzo-ingress \
  --providers.kubernetesingress.allowemptyservices=true \
  --entrypoints.web.address=:80 \
  --entrypoints.websecure.address=:443 \
  --entrypoints.websecure.http.tls=true \
  --ping=true \
  --ping.entryPoint=web \
  --api.dashboard=false \
  --log.level=INFO \
  --accesslog=true
```

### Configuration File

```toml
[entryPoints]
  [entryPoints.web]
    address = ":80"
  [entryPoints.websecure]
    address = ":443"
    [entryPoints.websecure.http.tls]

[providers]
  [providers.kubernetesIngress]
    [providers.kubernetesIngress.ingressEndpoint]
      publishedService = "hanzo/hanzo-ingress"

[ping]
  entryPoint = "web"

[log]
  level = "INFO"

[accessLog]
```

See the sample configuration files in the repository root for full examples.

## Repository Structure

```
cmd/                    # Binary entry point
internal/               # Core routing, middleware, provider logic
pkg/                    # Public packages and configuration types
webui/                  # Built-in dashboard (React)
k8s/
  hanzo/                # Production K8s manifests
    rbac.yaml           # ServiceAccount + ClusterRole
    ingressclass.yaml   # IngressClass "hanzo" (default)
    deployment.yaml     # 2-replica Deployment
    service.yaml        # LoadBalancer Service
integration/            # Integration test suite
contrib/                # Community contributed configs
docs/                   # Extended documentation
Dockerfile              # Multi-stage build (Node webui + Go binary)
Makefile                # Build, test, Docker targets
```

## PaaS Integration

Hanzo Ingress serves as the ingress layer for [Hanzo Platform](https://github.com/hanzoai/platform) (PaaS). Applications deployed through the platform automatically get:

- Ingress resource creation with proper host rules
- TLS certificate provisioning
- Load balancing across application replicas
- Access logging and metrics

## Hanzo Infrastructure Stack

Hanzo Ingress is one of four products in the Hanzo AI infrastructure stack:

| Product | Role | Repository |
|---------|------|------------|
| [**Hanzo Ingress**](https://github.com/hanzoai/ingress) | L7 reverse proxy, TLS termination, load balancing | `hanzoai/ingress` |
| [**Hanzo Gateway**](https://github.com/hanzoai/gateway) | API gateway, rate limiting, endpoint routing | `hanzoai/gateway` |
| [**Hanzo Engine**](https://github.com/hanzoai/engine) | GPU inference engine, model serving | `hanzoai/engine` |
| [**Hanzo Edge**](https://github.com/hanzoai/edge) | On-device inference runtime (mobile, web, embedded) | `hanzoai/edge` |

```
Internet -> Ingress (TLS/L7) -> Gateway (API routing) -> Engine (inference) / Cloud API / Services
                                                          Edge (on-device, client-side)
```

## License

MIT -- see [LICENSE.md](LICENSE.md).
