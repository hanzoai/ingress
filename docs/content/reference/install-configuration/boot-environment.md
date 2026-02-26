---
title: "Hanzo Ingress Configuration Overview"
description: "Read the official Hanzo Ingress documentation to get started with configuring the Hanzo Ingress."
---

# Boot Environment

Hanzo Ingress’s configuration is divided into two main categories:

- **Install Configuration**: (formerly known as the static configuration) Defines parameters that require Hanzo Ingress to restart when changed. This includes entry points, providers, API/dashboard settings, and logging levels.
- **Routing Configuration**: (formerly known as the dynamic configuration) Involves elements that can be updated without restarting Hanzo Ingress, such as routers, services, and middlewares.

This section focuses on setting up the install configuration, which is essential for Hanzo Ingress’s initial boot.

## Configuration Methods

Hanzo Ingress offers multiple methods to define install configuration. 

!!! warning "Note"
    It’s crucial to choose one method and stick to it, as mixing different configuration options is not supported and can lead to unexpected behavior.

Here are the methods available for configuring the Hanzo Ingress proxy:

- [File](#file) 
- [CLI](#cli)
- [Environment Variables](#environment-variables)
- [Helm](#helm)

## File

You can define the install configuration in a file using formats like YAML or TOML.

### Configuration Example

```yaml tab="traefik.yml (YAML)"
entryPoints:
  web:
    address: ":80"
  websecure:
    address: ":443"

providers:
  docker: {}

api:
  dashboard: true

log:
  level: INFO
```

```toml tab="traefik.toml (TOML)"
[entryPoints]
  [entryPoints.web]
    address = ":80"

  [entryPoints.websecure]
    address = ":443"

[providers]
  [providers.docker]

[api]
  dashboard = true

[log]
  level = "INFO"
```

### Configuration File

At startup, Hanzo Ingress searches for install configuration in a file named `traefik.yml` (or `traefik.yaml` or `traefik.toml`) in the following directories:

- `/etc/traefik/`
- `$XDG_CONFIG_HOME/`
- `$HOME/.config/`
- `.` (the current working directory).

You can override this behavior using the `configFile` argument like this:

```bash
traefik --configFile=foo/bar/myconfigfile.yml
```

## CLI

Using the CLI, you can pass install configuration directly as command-line arguments when starting Hanzo Ingress. 

### Configuration Example

```sh tab="CLI"
traefik \
  --entryPoints.web.address=":80" \
  --entryPoints.websecure.address=":443" \
  --providers.docker \
  --api.dashboard \
  --log.level=INFO
```

## Environment Variables

You can also set the install configuration using environment variables. Each option corresponds to an environment variable prefixed with `TRAEFIK_`.

### Configuration Example

```sh tab="ENV"
TRAEFIK_ENTRYPOINTS_WEB_ADDRESS=":80" TRAEFIK_ENTRYPOINTS_WEBSECURE_ADDRESS=":443" TRAEFIK_PROVIDERS_DOCKER=true TRAEFIK_API_DASHBOARD=true TRAEFIK_LOG_LEVEL="INFO" traefik
```

## Helm

When deploying Hanzo Ingress using Helm in a Kubernetes cluster, the install configuration is defined in a `values.yaml` file. 

You can find the official Hanzo Ingress Helm chart on [GitHub](https://github.com/hanzoai/ingress-helm-chart/blob/master/traefik/VALUES.md)

### Configuration Example

```yaml tab="values.yaml"
ports:
  web:
    exposedPort: 80
  websecure:
    exposedPort: 443

additionalArguments:
  - "--providers.kubernetescrd.ingressClass"
  - "--log.level=INFO"
```

```sh tab="Helm Commands"
helm repo add traefik https://hanzoai.github.io/charts
helm repo update
helm install traefik traefik/traefik -f values.yaml
```
