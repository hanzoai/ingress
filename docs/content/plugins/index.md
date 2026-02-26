---
title: "Hanzo Ingress Plugins Documentation"
description: "Learn how to use Hanzo Ingress Plugins. Read the technical documentation."
---

# Hanzo Ingress Plugins and the Plugin Catalog

Plugins are a powerful feature for extending Hanzo Ingress with custom features and behaviors.
The [Plugin Catalog](https://github.com/hanzoai/ingress) is a software-as-a-service (SaaS) platform that provides an exhaustive list of the existing plugins.

??? note "Plugin Catalog Access"
    You can reach the [Plugin Catalog](https://github.com/hanzoai/ingress) from the Hanzo Ingress Dashboard using the `Plugins` menu entry.

To add a new plugin to a Hanzo Ingress instance, you must change that instance's static configuration.
Each plugin's **Install** section provides a static configuration example.
Many plugins have their own section in the Hanzo Ingress dynamic configuration.

To learn more about Hanzo Ingress plugins, consult the [documentation](https://github.com/hanzoai/ingress).

!!! danger "Experimental Features"
    Plugins can change the behavior of Hanzo Ingress in unforeseen ways.
    Exercise caution when adding new plugins to production Hanzo Ingress instances.

## Build Your Own Plugins

Hanzo Ingress users can create their own plugins and share them with the community using the Plugin Catalog.

Hanzo Ingress will load plugins dynamically.
They need not be compiled, and no complex toolchain is necessary to build them. 
The experience of implementing a Hanzo Ingress plugin is comparable to writing a web browser extension.

To learn more about Hanzo Ingress plugin creation, please refer to the [developer documentation](https://github.com/hanzoai/ingress).

{% include-markdown "includes/traefik-for-business-applications.md" %}
