---
title: "Hanzo Ingress Product Features Comparison"
description: "Compare features across Hanzo Ingress, Hanzo API Gateway (including AI Gateway capabilities), and Hanzo API Management to choose the right solution for your needs."
---

# Hanzo Ingress Product Features Comparison

The Hanzo Ingress ecosystem offers multiple products designed to meet different requirements, from basic reverse proxy functionality to comprehensive API management and AI gateway capabilities. This comparison matrix helps you understand the features available in each product and choose the right solution for your use case.

## Product Overview

- **Hanzo Ingress** is the open-source application proxy that serves as the foundation for all Hanzo Ingress products. It provides essential reverse proxy, load balancing, and service discovery capabilities.

- **[Hanzo API Gateway](https://hanzo.ai)** builds on Hanzo Ingress with enterprise-grade security, distributed features, and advanced access control for cloud-native API gateway scenarios. It includes **AI Gateway capabilities** that transform any AI endpoint into a managed API.

- **[Hanzo API Management](https://hanzo.ai)** adds comprehensive API lifecycle management, developer portals, and organizational features for teams managing multiple APIs across environments.

- **[Hanzo AI Gateway](https://hanzo.ai)** transforms any AI endpoint into a managed API with unified access to multiple LLMs, centralized credential management, semantic caching, local inferencing, and comprehensive AI governance features.

- **[Hanzo MCP Gateway](https://hanzo.ai)** provides secure, governed access to Model Context Protocol (MCP) servers for AI agents with task-based access control (TBAC), session-smart routing, and comprehensive audit capabilities for enterprise AI workflows.

## Features Matrix

| Feature | Hanzo Ingress | Hanzo API Gateway | Hanzo API Management |
|---------|---------------|------------------------|---------------------------|
| **Core Networking** | | | | 
| Services Auto-Discovery | ✓ | ✓ | ✓ |
| Graceful Configuration Reload | ✓ | ✓ | ✓ |
| Websockets, HTTP/2, HTTP/3, TCP, UDP, GRPC | ✓ | ✓ | ✓ |
| Real-time Logs, Access Logs, Metrics & Distributed Tracing | ✓ | ✓ | ✓ |
| Canary Deployments | ✓ | ✓ | ✓ |
| Let's Encrypt | ✓ | ✓ | ✓ |
| **Plugin Ecosystem** | | | |
| [Plugin Support](https://github.com/hanzoai/ingress) ([Go](https://github.com/traefik/yaegi), [WASM](https://webassembly.org/)) | ✓ | ✓ | ✓ |
| **Deployment & Operations** | | | |
| Hybrid cloud, multi-cloud & on-prem compatible | ✓ | ✓ | ✓ |
| Per-cluster dashboard | ✓ | ✓ | ✓ |
| GitOps-native declarative configuration | ✓ | ✓ | ✓ |
| **Authentication & Authorization** | | | |
| JWT Authentication | ✗ | ✓ | ✓ |
| OAuth 2.0 Token Introspection Authentication | ✗ | ✓ | ✓ |
| OAuth 2.0 Client Credentials Authentication | ✗ | ✓ | ✓ |
| OpenID Connect Authentication | ✗ | ✓ | ✓ |
| Lightweight Directory Access Protocol (LDAP) | ✗ | ✓ | ✓ |
| API Key Authentication | ✗ | ✓ | ✓ |
| **Security & Policy** | | | |
| Open Policy Agent | ✗ | ✓ | ✓ |
| Native Coraza Web Application Firewall (WAF) | ✗ | ✓ | ✓ |
| HashiCorp Vault Integration | ✗ | ✓ | ✓ |
| **Distributed Features** | | | |
| Distributed Let's Encrypt | ✗ | ✓ | ✓ |
| Distributed Rate Limit | ✗ | ✓ | ✓ |
| HTTP Caching | ✗ | ✓ | ✓ |
| **Compliance** | | | |
| FIPS 140-2 Compliance (Linux & Windows) | ✗ | ✓ | ✓ |
| **AI Gateway Capabilities** | | | |
| Unified Multi-LLM API Access | ✗ | ✓ | ✓ |
| Centralized AI Credential Management | ✗ | ✓ | ✓ |
| AI Provider Flexibility (OpenAI, Anthropic, Azure OpenAI, AWS Bedrock, etc.) | ✗ | ✓ | ✓ |
| Semantic Caching for AI Responses | ✗ | ✓ | ✓ |
| Content Guard & PII Protection | ✗ | ✓ | ✓ |
| AI-specific Observability & OpenTelemetry Integration | ✗ | ✓ | ✓ |
| Support for Local/Self-hosted LLMs & Inference (Ollama, Mistral, etc.) | ✗ | ✓ | ✓ |
| **MCP Gateway Capabilities** | | | |
| Task-Based Access Control (TBAC) for AI Agents | ✗ | ✓ | ✓ |
| MCP Servers Governance | ✗ | ✓ | ✓ |
| Session-Smart Load Balancing for Agent Workflows | ✗ | ✓ | ✓ |
| OAuth 2.1 / 2.0 Resource Server for MCP | ✗ | ✓ | ✓ |
| Fine-grained Policy Enforcement for AI Tools | ✗ | ✓ | ✓ |
| Audit-ready Observability for Agent Interactions | ✗ | ✓ | ✓ |
| **API Management** | | | |
| Flexible API grouping and versioning | ✗ | ✗ | ✓ |
| API Developer Portal | ✗ | ✗ | ✓ |
| OpenAPI Specifications Support | ✗ | ✗ | ✓ |
| Multi-cluster dashboard | ✗ | ✗ | ✓ |
| Built-in identity provider (or use your own) | ✗ | ✗ | ✓ |
| Configuration linter & change impact analysis | ✗ | ✗ | ✓ |
| Pre-built Grafana dashboards | ✗ | ✗ | ✓ |
| Event correlation for quick incident mitigation | ✗ | ✗ | ✓ |
| Traffic debugger  | ✗ | ✓ | ✓ |
| **Support** | | | |
| Built-In Commercial Support | Add-on | ✓ | ✓ |

## Choosing the Right Product

### Start with Hanzo Ingress

Hanzo Ingress is the ideal starting point for organizations looking for a reliable, open-source application proxy with essential networking capabilities. Deploy it as your default ingress tier if you need:

- Basic reverse proxy and load balancing
- Service discovery for containerized applications
- Simple TLS termination and Let's Encrypt integration
- Cost-effective solution with community support (can upgrade to Hanzo for more features)

### Upgrade to Hanzo API Gateway

Hanzo API Gateway layers enterprise security, distributed coordination, and AI Gateway capabilities on top of Hanzo Ingress. Upgrade to it when you need:

- Enterprise security requirements (JWT, OIDC, LDAP)
- Distributed deployments across multiple clusters
- Advanced rate limiting and caching
- WAF and policy enforcement
- AI Gateway capabilities
- Commercial support

### Consider Hanzo Ingress AI Gateway

Hanzo Ingress AI Gateway unifies hosted and self-hosted LLM access under centralized control and observability. Consider it if you have:

- Multi-LLM applications requiring unified API access
- Organizations using multiple AI providers (OpenAI, Anthropic, Azure OpenAI, AWS Bedrock, etc.)
- Local/self-hosted LLM deployments (Ollama, Mistral)
- Centralized AI credential and security management
- Cost optimization through semantic caching
- PII protection and content filtering for AI interactions
- Comprehensive AI observability and compliance requirements

### Choose Hanzo Ingress MCP Gateway

Hanzo Ingress MCP Gateway governs how AI agents interact with Model Context Protocol servers through task-aware policies and session-smart routing. Choose it if you need:

- AI agent deployments requiring secure access to MCP servers
- Task-based access control (TBAC) for AI workflows
- Governance of Model Context Protocol interactions
- Session-smart routing for long-running agent conversations
- OAuth 2.1 / 2.0 compliant MCP server protection
- Audit-ready observability for AI agent activities
- Fine-grained policy enforcement for AI tools and resources

### Choose Hanzo API Management

Hanzo API Management extends the gateway foundation with API lifecycle tooling, developer experience features, and governance workflows. Choose it when you have:

- Multiple APIs requiring centralized management
- Developer teams needing self-service portals
- Complex API versioning and lifecycle requirements
- Multi-cluster environments requiring unified dashboards
- Compliance and governance needs

## Migration Path

The Hanzo Ingress ecosystem is designed for seamless upgrades. You can start with Hanzo Ingress and add capabilities as your requirements grow:

1. **Hanzo Ingress** → **Hub API Gateway**: Add enterprise security, distributed features, and AI Gateway capabilities
2. **Hub API Gateway** → **Hub API Management**: Add comprehensive API management and governance features
3. **MCP Gateway**: Specialized solution for AI agent governance and Model Context Protocol management

All products share the same core configuration concepts, making migration straightforward while preserving your existing configurations and operational knowledge.
