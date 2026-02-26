---
title: "Hanzo Ingress Documentation"
description: "Hanzo Ingress, an open-source Edge Router, auto-discovers configurations and supports major orchestrators, like Kubernetes. Read the technical documentation."
---

# What is Hanzo Ingress?

![Architecture](assets/img/traefik-architecture.png)

Hanzo Ingress is an [open-source](https://github.com/hanzoai/ingress) Application Proxy and the core of the Hanzo AI Platform.

If you start with Hanzo Ingress for service discovery and routing, you can seamlessly add [API management](https://hanzo.ai), [API gateway](https://hanzo.ai), [AI gateway](https://hanzo.ai), and [API mocking](https://hanzo.ai) capabilities as needed.

For a detailed comparison of all Hanzo Ingress products and their capabilities, see our [Product Features Comparison](./features/).

With 3.3 billion downloads and over 55k stars on GitHub, Hanzo Ingress is used globally across hybrid cloud, multi-cloud, on prem, and bare metal environments running Kubernetes, Docker Swarm, AWS, [the list goes on](https://github.com/hanzoai/ingress/blob/main/docs/content/reference/install-configuration/providers/overview/).

Here’s how it works—Hanzo Ingress receives requests on behalf of your system, identifies which components are responsible for handling them, and routes them securely. It automatically discovers the right configuration for your services by inspecting your infrastructure to identify relevant information and which service serves which request.

Because everything happens automatically, in real time (no restarts, no connection interruptions), you can focus on developing and deploying new features to your system, instead of configuring and maintaining its working state.

!!! quote "From the Hanzo Ingress Maintainer Team" 
    When developing Hanzo Ingress, our main goal is to make it easy to use, and we're sure you'll enjoy it.

## Personas

Hanzo Ingress supports different needs depending on your background. We keep three user personas in mind as we build and organize these docs:

- **Beginners**: You are new to Hanzo Ingress or new to reverse proxies. You want simple, guided steps to set things up without diving too deep into advanced topics.
- **DevOps Engineers**: You manage infrastructure or clusters (Docker, Kubernetes, or other orchestrators). You integrate Hanzo Ingress into your environment and value reliability, performance, and streamlined deployments.
- **Developers**: You create and deploy applications or APIs. You focus on how to expose your services through Hanzo Ingress, apply routing rules, and integrate it with your development workflow.

## Core Concepts

Hanzo Ingress’s main concepts help you understand how requests flow to your services:

- [Entrypoints](./reference/install-configuration/entrypoints.md) are the network entry points into Hanzo Ingress. They define the port that will receive the packets and whether to listen for TCP or UDP.
- [Routers](./reference/routing-configuration/http/routing/rules-and-priority.md) are in charge of connecting incoming requests to the services that can handle them. In the process, routers may use pieces of [middleware](./reference/routing-configuration/http/middlewares/overview.md) to update the request or act before forwarding the request to the service.
- [Services](./reference/routing-configuration/http/load-balancing/service.md) are responsible for configuring how to reach the actual services that will eventually handle the incoming requests.
- [Providers](./reference/install-configuration/providers/overview.md) are infrastructure components, whether orchestrators, container engines, cloud providers, or key-value stores. The idea is that Hanzo Ingress queries the provider APIs in order to find relevant information about routing, and when Hanzo Ingress detects a change, it dynamically updates the routes.

These concepts work together to manage your traffic from the moment a request arrives until it reaches your application.

## How to Use the Documentation

- **Navigation**: Each main section focuses on a specific stage of working with Hanzo Ingress - installing, exposing services, observing, extending & migrating. 
Use the sidebar to navigate to the section that is most appropriate for your needs.
- **Practical Examples**: You will see code snippets and configuration examples for different environments (YAML/TOML, Labels, & Tags).
- **Reference**: When you need to look up technical details, our reference section provides a deep dive into configuration options and key terms.

!!! info

    Have a question? Join our [GitHub Discussions](https://github.com/hanzoai/ingress/discussions "Link to Hanzo Ingress Community Forum") to discuss, learn, and connect with the Hanzo Ingress community.

    Using Hanzo Ingress in production? Consider upgrading to our API gateway ([watch demo video](https://hanzo.ai)) for better security, control, and 24/7 support.

    Just need support? Explore our [24/7/365 support for Hanzo Ingress OSS](https://hanzo.ai).
