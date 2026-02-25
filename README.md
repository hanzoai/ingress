# Hanzo Ingress

Cloud-native reverse proxy and load balancer for Hanzo infrastructure. Fork of [Traefik](https://github.com/traefik/traefik).

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/hanzoai/ingress/blob/main/LICENSE.md)

## Features

- Automatic service discovery and configuration (Docker, Kubernetes, ECS, Consul, Etcd)
- Dynamic configuration updates with zero restarts
- Automatic HTTPS via Let's Encrypt (wildcard support)
- HTTP/2, gRPC, WebSocket support
- Built-in circuit breakers and retry logic
- Metrics export (Prometheus, Datadog, StatsD, InfluxDB, OTLP)
- Access logging (JSON, CLF)
- Web dashboard UI
- REST API
- Single static binary

## Quick Start

```bash
# Build
make build

# Run
./hanzo-ingress --configFile=traefik.toml

# Docker
make docker
docker run -d -p 8080:8080 -p 80:80 ghcr.io/hanzoai/ingress:latest
```

## Documentation

Upstream documentation: [doc.traefik.io/traefik](https://doc.traefik.io/traefik/)

## License

MIT - see [LICENSE.md](LICENSE.md)
