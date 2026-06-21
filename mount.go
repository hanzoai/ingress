// Copyright © 2026 Hanzo AI. MIT License.

//go:build cloud
// +build cloud

// Package ingress exposes the HIP-0106 unified-binary mount surface for
// Hanzo Ingress.
//
// Architecturally, the full reverse-proxy / TLS / provider machinery
// lives in `cmd/ingress` and stays out-of-process at the cluster edge —
// nothing about that changes. What Mount provides is the small
// in-process slice the unified cloud binary needs: a healthz endpoint
// (so probes work even when the edge proxy is reloading) and a
// read-only /_/ingress/config view of which providers + entrypoints
// the deployment expects. This lets cloud-mounted subsystems verify
// that an ingress is configured for the brand without standing the
// full proxy up inside the cloud binary.
package ingress

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/hanzoai/cloud"
	"github.com/zap-proto/zip"
)

// Mount registers ingress on app per HIP-0106. Order 90 — runs after
// gateway (80) so gateway's identity middleware is already installed
// when our health endpoint serves.
//
// The mount is intentionally LIGHT. Hanzo Ingress is k8s-native: a
// separate `hanzo-ingress` binary runs at the cluster edge with
// providers (Kubernetes, file, docker, etc.) discovering routes. The
// unified cloud binary co-resident with Ingress benefits from the
// /_/ingress/healthz endpoint for probe coverage and the
// /_/ingress/config endpoint to confirm the deployment's expected
// entrypoint set — nothing else here.
func Mount(app *zip.App, deps cloud.Deps) error {
	if app == nil {
		return fmt.Errorf("ingress.Mount: nil zip.App")
	}
	logger := deps.Logger
	if logger == nil {
		return fmt.Errorf("ingress.Mount: nil deps.Logger")
	}

	cfg := configFromEnv(deps)

	app.Get("/_/ingress/healthz", func(c *zip.Ctx) error {
		return c.JSON(http.StatusOK, map[string]any{
			"status":  "ok",
			"service": "ingress",
		})
	})

	app.Get("/_/ingress/config", func(c *zip.Ctx) error {
		// Read-only view: brand, domain, expected entrypoints. No
		// provider credentials, no TLS material, no upstream URLs.
		return c.JSON(http.StatusOK, map[string]any{
			"brand":        deps.Brand,
			"domain":       deps.Domain,
			"entrypoints":  cfg.Entrypoints,
			"providers":    cfg.Providers,
			"acme_enabled": cfg.ACMEEnabled,
		})
	})

	logger.Info("ingress mounted",
		"brand", deps.Brand,
		"domain", deps.Domain,
		"entrypoints", strings.Join(cfg.Entrypoints, ","),
		"providers", strings.Join(cfg.Providers, ","),
		"acme_enabled", cfg.ACMEEnabled,
		"json_variant", zip.JSONVariant,
	)
	return nil
}

// runtimeConfig is a tiny env-driven projection of the full Ingress
// static config. We do NOT import pkg/config/static into the cloud
// build path — that pulls in the entire provider tree (Kubernetes,
// Docker, AWS, Consul, …). The full config still lives in
// cmd/ingress/ingress.go for the standalone binary.
type runtimeConfig struct {
	Entrypoints []string
	Providers   []string
	ACMEEnabled bool
}

func configFromEnv(deps cloud.Deps) runtimeConfig {
	cfg := runtimeConfig{
		Entrypoints: splitCSV(getenv("INGRESS_ENTRYPOINTS", "web,websecure")),
		Providers:   splitCSV(getenv("INGRESS_PROVIDERS", "kubernetes")),
		ACMEEnabled: getenv("INGRESS_ACME_ENABLED", "true") == "true",
	}
	_ = deps
	return cfg
}

func init() {
	cloud.Register("ingress", 90, func(app any, deps cloud.Deps) error {
		zapp, ok := app.(*zip.App)
		if !ok {
			return fmt.Errorf("ingress.Mount: expected *zip.App, got %T", app)
		}
		return Mount(zapp, deps)
	})
}

func getenv(key, dflt string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return dflt
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
