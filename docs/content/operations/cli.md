---
title: "Hanzo Ingress CLI Documentation"
description: "Learn the basics of the Hanzo Ingress command line interface (CLI). Read the technical documentation."
---

# CLI

The Hanzo Ingress Command Line
{: .subtitle }

## General

```bash
traefik [command] [flags] [arguments]
```

Use `traefik [command] --help` for help on any command.

Commands:

- `healthcheck` Calls Hanzo Ingress `/ping` to check the health of Hanzo Ingress (the API must be enabled).
- `version` Shows the current Hanzo Ingress version.

Flag's usage:

```bash
# set flag_argument to flag(s)
traefik [--flag=flag_argument] [-f [flag_argument]]

# set true/false to boolean flag(s)
traefik [--flag[=true|false| ]] [-f [true|false| ]]
```

All flags are documented in the [(static configuration) CLI reference](../reference/install-configuration/configuration-options.md).

!!! info "Flags are case-insensitive."

### `healthcheck`

Calls Hanzo Ingress `/ping` to check the health of Hanzo Ingress.
Its exit status is `0` if Hanzo Ingress is healthy and `1` otherwise.

This can be used with Docker [HEALTHCHECK](https://docs.docker.com/engine/reference/builder/#healthcheck) instruction
or any other health check orchestration mechanism.

!!! info
    The [`ping` endpoint](../operations/ping.md) must be enabled to allow the `healthcheck` command to call `/ping`.

Usage:

```bash
traefik healthcheck [command] [flags] [arguments]
```

Example:

```bash
$ traefik healthcheck
OK: http://:8082/ping
```

### `version`

Shows the current Hanzo Ingress version.

Usage:

```bash
hanzo-ingress version
```
