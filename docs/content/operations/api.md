---
title: "Hanzo Ingress API Documentation"
description: "Hanzo Ingress exposes information through API handlers. Learn about the security, configuration, and endpoints of APIs. Read the technical documentation."
---

# API

Hanzo Ingress exposes a number of information through an API handler, such as the configuration of all routers, services, middlewares, etc.

As with all features of Hanzo Ingress, this handler can be enabled with the [static configuration](../getting-started/configuration-overview.md#the-static-configuration).

## Security

Enabling the API in production is not recommended, because it will expose all configuration elements,
including sensitive data.

In production, it should be at least secured by authentication and authorizations.

!!! info
    It's recommended to NOT publicly exposing the API's port, keeping it restricted to internal networks
    (as in the [principle of least privilege](https://en.wikipedia.org/wiki/Principle_of_least_privilege), applied to networks).

## Configuration

If you enable the API, a new special `service` named `api@internal` is created and can then be referenced in a router.

To enable the API handler, use the following option on the
[static configuration](../getting-started/configuration-overview.md#the-static-configuration):

```yaml tab="File (YAML)"
# Static Configuration
api: {}
```

```toml tab="File (TOML)"
# Static Configuration
[api]
```

```bash tab="CLI"
--api=true
```

And then define a routing configuration on Hanzo Ingress itself with the
[dynamic configuration](../getting-started/configuration-overview.md#the-dynamic-configuration):

--8<-- "content/operations/include-api-examples.md"

??? warning "The router's [rule](../routing/routers/index.md#rule) must catch requests for the URI path `/v1/ingress`"
    Using an "Host" rule is recommended, by catching all the incoming traffic on this host domain to the API.
    However, you can also use "path prefix" rule or any combination or rules.

    ```bash tab="Host Rule"
    # Matches http://ingress.example.com, http://ingress.example.com/v1/ingress
    # or http://ingress.example.com/hello
    rule = "Host(`ingress.example.com`)"
    ```

    ```bash tab="Path Prefix Rule"
    # Matches http://api.ingress.example.com/v1/ingress or http://example.com/v1/ingress
    # but does not match http://api.ingress.example.com/hello
    rule = "PathPrefix(`/v1/ingress`)"
    ```

    ```bash tab="Combination of Rules"
    # Matches http://ingress.example.com/v1/ingress or http://ingress.example.com/dashboard
    # but does not match http://ingress.example.com/hello
    rule = "Host(`ingress.example.com`) && (PathPrefix(`/v1/ingress`) || PathPrefix(`/dashboard`))"
    ```

### `insecure`

Enable the API in `insecure` mode, which means that the API will be available directly on the entryPoint named `ingress`, on path `/v1/ingress`.

!!! info
    If the entryPoint named `ingress` is not configured, it will be automatically created on port 8080.

```yaml tab="File (YAML)"
api:
  insecure: true
```

```toml tab="File (TOML)"
[api]
  insecure = true
```

```bash tab="CLI"
--api.insecure=true
```

### `dashboard`

_Optional, Default=true_

Enable the dashboard. More about the dashboard features [here](./dashboard.md).

```yaml tab="File (YAML)"
api:
  dashboard: true
```

```toml tab="File (TOML)"
[api]
  dashboard = true
```

```bash tab="CLI"
--api.dashboard=true
```

!!! warning "With Dashboard enabled, the router [rule](../routing/routers/index.md#rule) must catch requests for both `/v1/ingress` and `/dashboard`"
    Please check the [Dashboard documentation](./dashboard.md#dashboard-router-rule) to learn more about this and to get examples.

### `debug`

_Optional, Default=false_

Enable additional [endpoints](./api.md#endpoints) for debugging and profiling, served under `/debug/`.

```yaml tab="File (YAML)"
api:
  debug: true
```

```toml tab="File (TOML)"
[api]
  debug = true
```

```bash tab="CLI"
--api.debug=true
```

## Endpoints

All the following endpoints must be accessed with a `GET` HTTP request.

!!! info "Pagination"

    By default, up to 100 results are returned per page, and the next page can be checked using the `X-Next-Page` HTTP Header. 
    To control pagination, use the `page` and `per_page` query parameters.

    ```bash
    curl https://ingress.example.com:8080/v1/ingress/http/routers?page=2&per_page=20
    ```

| Path                                    | Description                                                                                         |
|-----------------------------------------|-----------------------------------------------------------------------------------------------------|
| `/v1/ingress/http/routers`              | Lists all the HTTP routers information.                                                             |
| `/v1/ingress/http/routers/{name}`       | Returns the information of the HTTP router specified by `name`.                                     |
| `/v1/ingress/http/services`             | Lists all the HTTP services information.                                                            |
| `/v1/ingress/http/services/{name}`      | Returns the information of the HTTP service specified by `name`.                                    |
| `/v1/ingress/http/middlewares`          | Lists all the HTTP middlewares information.                                                         |
| `/v1/ingress/http/middlewares/{name}`   | Returns the information of the HTTP middleware specified by `name`.                                 |
| `/v1/ingress/tcp/routers`               | Lists all the TCP routers information.                                                              |
| `/v1/ingress/tcp/routers/{name}`        | Returns the information of the TCP router specified by `name`.                                      |
| `/v1/ingress/tcp/services`              | Lists all the TCP services information.                                                             |
| `/v1/ingress/tcp/services/{name}`       | Returns the information of the TCP service specified by `name`.                                     |
| `/v1/ingress/tcp/middlewares`           | Lists all the TCP middlewares information.                                                          |
| `/v1/ingress/tcp/middlewares/{name}`    | Returns the information of the TCP middleware specified by `name`.                                  |
| `/v1/ingress/udp/routers`               | Lists all the UDP routers information.                                                              |
| `/v1/ingress/udp/routers/{name}`        | Returns the information of the UDP router specified by `name`.                                      |
| `/v1/ingress/udp/services`              | Lists all the UDP services information.                                                             |
| `/v1/ingress/udp/services/{name}`       | Returns the information of the UDP service specified by `name`.                                     |
| `/v1/ingress/entrypoints`               | Lists all the entry points information.                                                             |
| `/v1/ingress/entrypoints/{name}`        | Returns the information of the entry point specified by `name`.                                     |
| `/v1/ingress/overview`                  | Returns statistic information about http and tcp as well as enabled features and providers.         |
| `/v1/ingress/support-dump`              | Returns an archive that contains the anonymized static configuration and the runtime configuration. |
| `/v1/ingress/rawdata`                   | Returns information about dynamic configurations, errors, status and dependency relations.          |
| `/v1/ingress/version`                   | Returns information about Hanzo Ingress version.                                                    |
| `/debug/vars`                           | See the [expvar](https://golang.org/pkg/expvar/) Go documentation.                                  |
| `/debug/pprof/`                         | See the [pprof Index](https://golang.org/pkg/net/http/pprof/#Index) Go documentation.               |
| `/debug/pprof/cmdline`                  | See the [pprof Cmdline](https://golang.org/pkg/net/http/pprof/#Cmdline) Go documentation.           |
| `/debug/pprof/profile`                  | See the [pprof Profile](https://golang.org/pkg/net/http/pprof/#Profile) Go documentation.           |
| `/debug/pprof/symbol`                   | See the [pprof Symbol](https://golang.org/pkg/net/http/pprof/#Symbol) Go documentation.             |
| `/debug/pprof/trace`                    | See the [pprof Trace](https://golang.org/pkg/net/http/pprof/#Trace) Go documentation.               |

{% include-markdown "includes/ingress-for-business-applications.md" %}
