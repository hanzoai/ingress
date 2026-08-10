// Copyright © 2026 Hanzo AI. MIT License.

// Package ingress is the in-process slice of Hanzo Ingress.
//
// The reverse-proxy / TLS / provider machinery lives in cmd/ingress and stays
// out-of-process at the cluster edge — nothing about that changes here. What
// this package builds is the small surface a co-resident binary needs: a health
// endpoint that answers while the edge proxy reloads, and a read-only
// /_/ingress/config view of the entrypoints and providers the deployment
// expects. That is enough to confirm an ingress is configured for the brand
// without standing the whole proxy up in the same process.
package ingress

import (
	"net/http"
	"os"
	"strings"

	luxlog "github.com/luxfi/log"
	"github.com/zap-proto/zip"
)

// App builds ingress's in-process surface and returns it, for a host to
// compose.
//
// Brand and domain are the only two facts ingress needs from whoever runs it,
// and both are read straight back out on /_/ingress/config, so they arrive as
// two strings. They used to arrive as a cloud.Deps — the host's own dependency
// struct, carried across the boundary to reach two fields. That made ingress
// import its host, which costs more than the import line: hanzoai/cloud ships
// two editions that both declare the module path github.com/hanzoai/cloud, so
// cloud.Deps names a different type in each, and a package that takes one can
// be composed by exactly one of them.
//
// Returning the app rather than writing into one handed in turns the last arrow
// around too. Ingress no longer registers itself in a registry the host owns —
// the host holds what it gets back and mounts it wherever it likes.
func App(brand, domain string) (*zip.App, error) {
	app := zip.New(zip.Config{AppName: "ingress"})
	cfg := configFromEnv()

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
			"brand":        brand,
			"domain":       domain,
			"entrypoints":  cfg.Entrypoints,
			"providers":    cfg.Providers,
			"acme_enabled": cfg.ACMEEnabled,
		})
	})

	luxlog.Default().Info("ingress mounted",
		"brand", brand,
		"domain", domain,
		"entrypoints", strings.Join(cfg.Entrypoints, ","),
		"providers", strings.Join(cfg.Providers, ","),
		"acme_enabled", cfg.ACMEEnabled,
		"json_variant", zip.JSONVariant,
	)
	return app, nil
}

// runtimeConfig is a tiny env-driven projection of the full Ingress
// static config. We do NOT import pkg/config/static into this build
// path — that pulls in the entire provider tree (Kubernetes, Docker,
// AWS, Consul, …). The full config still lives in cmd/ingress/ingress.go
// for the standalone binary.
type runtimeConfig struct {
	Entrypoints []string
	Providers   []string
	ACMEEnabled bool
}

func configFromEnv() runtimeConfig {
	return runtimeConfig{
		Entrypoints: splitCSV(getenv("INGRESS_ENTRYPOINTS", "web,websecure")),
		Providers:   splitCSV(getenv("INGRESS_PROVIDERS", "kubernetes")),
		ACMEEnabled: getenv("INGRESS_ACME_ENABLED", "true") == "true",
	}
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
