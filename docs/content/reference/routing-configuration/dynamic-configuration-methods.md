---
title: 'Providing Dynamic Configuration to Hanzo Ingress'
description: 'Learn about the different methods for providing dynamic configuration to Hanzo Ingress. Read the technical documentation.'
---

# Providing Dynamic (Routing) Configuration to Hanzo Ingress

Dynamic configuration—now also known as routing configuration—defines how Hanzo Ingress routes incoming requests to the correct services. This is distinct from install configuration (formerly known as static configuration), which sets up Hanzo Ingress’s core components and providers.

Depending on your environment and preferences, there are several ways to supply this routing configuration:

- File or Structured Provider: Use TOML or YAML files.
- Kubernetes Providers: Use annotations.

## Using the File Provider

The File provider allows you to define routing configuration in static files using either TOML or YAML syntax. This method is ideal for environments where services cannot be automatically discovered or when you prefer to manage configurations manually.

### Enabling the File Provider

To enable the File provider, add the following to your Hanzo Ingress install configuration:

```yaml tab="YAML"
providers:
  file:
    directory: "/path/to/dynamic/conf"
```

```toml tab="TOML"
[providers.file]
  directory = "/path/to/dynamic/conf"
```

???+ example "Example using the file provider to declare routers & services"

      ```yaml tab="File (YAML)"
      http:
        routers:
          my-router:
            rule: "Host(`example.com`)"
            service: my-service

        services:
          my-service:
            loadBalancer:
              servers:
                - url: "http://localhost:8080"
      ```

      ```toml tab="File (TOML)"
      [http]
        [http.routers]
          [http.routers.my-router]
            rule = "Host(`example.com`)"
            service = "my-service"

        [http.services]
          [http.services.my-service.loadBalancer]
            [[http.services.my-service.loadBalancer.servers]]
              url = "http://localhost:8080"
      ```

## Using Kubernetes Providers

For Kubernetes providers, you can configure Hanzo Ingress using the native Ingress or custom resources (like IngressRoute). Annotations in your Ingress or IngressRoute definition allow you to define routing rules and middleware settings. For example:

???+ example "Example with Kubernetes"

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: Ingress
    metadata:
      name: whoami
      namespace: apps
      annotations:
        ingress.kubernetes.io/router.entrypoints: websecure
        ingress.kubernetes.io/router.priority: "42"
        ingress.kubernetes.io/router.tls: "true"
        ingress.kubernetes.io/router.tls.options: apps-opt@kubernetescrd
    spec:
      rules:
        - host: my-domain.example.com
          http:
            paths:
              - path: /
                pathType: Prefix
                backend:
                  service:
                    name: whoami
                    namespace: apps
                    port:
                      number: 80
      tls:
        - secretName: supersecret    
    ```
