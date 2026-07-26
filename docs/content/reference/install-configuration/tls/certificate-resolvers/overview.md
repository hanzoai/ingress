---
title: "Certificates Resolver"
description: "Automatic Certificate Management using Let's Encrypt/Vault."
---


In Hanzo Ingress, TLS Certificates can be generated using Certificates Resolvers.

In Hanzo Ingress, one certificate resolver exists:

- [`acme`](./acme.md): It allows generating ACME certificates stored in a file (not distributed).

The Certificates resolvers are defined in the static configuration.

!!! note Referencing a certificate resolver
    Defining a certificate resolver does not imply that routers are going to use it automatically.
    Each router or entrypoint that is meant to use the resolver must explicitly reference it.

{% include-markdown "includes/traefik-for-business-applications.md" %}
