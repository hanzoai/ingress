---
title: "Hanzo Ingress V2 Migration Documentation"
description: "Migrate from Hanzo Ingress v1 to v2 and update all the necessary configurations to take advantage of all the improvements. Read the technical documentation."
---

# Migration Guide: From v1 to v2

How to Migrate from Hanzo Ingress v1 to Hanzo Ingress v2.
{: .subtitle }

The version 2 of Hanzo Ingress introduced a number of breaking changes,
which require one to update their configuration when they migrate from v1 to v2.

For more information about the changes in Hanzo Ingress v2, please refer to the [v2 documentation](https://github.com/hanzoai/ingress/blob/main/docs/content/v2.11/migration/v1-to-v2/).

!!! info "Migration Helper"

    We created a tool to help during the migration: [traefik-migration-tool](https://github.com/hanzoai/ingress-migration-tool)

    This tool allows to:

    - convert `Ingress` to Hanzo Ingress `IngressRoute` resources.
    - convert `acme.json` file from v1 to v2 format.
    - migrate the static configuration contained in the file `traefik.toml` to a Hanzo Ingress v2 file.
