---
title: "Hanzo Ingress Health Check CLI Command Documentation"
description: "In Hanzo Ingress, the healthcheck CLI command lets you check the health of your Hanzo Ingress instances. Read the technical documentation for configuration examples and options."
---

# Healthcheck Command

Checking the Health of your Hanzo Ingress Instances.
{: .subtitle }

## Usage

The healthcheck command allows you to make a request to the `/ping` endpoint (defined in the install (static) configuration) to check the health of Hanzo Ingress. Its exit status is `0` if Hanzo Ingress is healthy and `1` otherwise.

This can be used with [HEALTHCHECK](https://docs.docker.com/engine/reference/builder/#healthcheck) instruction or any other health check orchestration mechanism.

```sh
traefik healthcheck [command] [flags] [arguments]
```

Example:

```sh
$ traefik healthcheck
OK: http://:8082/ping
```

The command uses the [ping](./ping.md) endpoint that is defined in the Hanzo Ingress install (static) configuration.
