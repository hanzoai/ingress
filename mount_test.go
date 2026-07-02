// Copyright © 2026 Hanzo AI. MIT License.

//go:build cloud
// +build cloud

package ingress

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hanzoai/cloud"
	luxlog "github.com/luxfi/log"

	"github.com/zap-proto/zip"
)

func TestMount_RegistersHealthAndConfig(t *testing.T) {
	if !registryContains("ingress") {
		t.Fatalf("cloud.Registry missing 'ingress'; Names=%v", registryNames())
	}

	t.Setenv("INGRESS_ENTRYPOINTS", "web,websecure")
	t.Setenv("INGRESS_PROVIDERS", "kubernetes,file")
	t.Setenv("INGRESS_ACME_ENABLED", "true")

	app := zip.New(zip.Config{Logger: luxlog.New("test")})
	deps := cloud.Deps{
		Logger:  luxlog.New("test"),
		Brand:   "hanzo",
		Domain:  "api.hanzo.ai",
		DataDir: t.TempDir(),
	}
	if err := Mount(app, deps); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	for _, tc := range []struct {
		path   string
		want   string
		status int
	}{
		{"/_/ingress/healthz", `"service":"ingress"`, 200},
		{"/_/ingress/config", `"brand":"hanzo"`, 200},
	} {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			resp, err := app.Fiber().Test(req)
			if err != nil {
				t.Fatalf("Fiber Test: %v", err)
			}
			if resp.StatusCode != tc.status {
				t.Fatalf("status: got %d want %d", resp.StatusCode, tc.status)
			}
			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if !strings.Contains(string(raw), tc.want) {
				t.Fatalf("body: got %q, want substring %q", string(raw), tc.want)
			}
		})
	}
}

func registryContains(name string) bool {
	for _, s := range cloud.Registry {
		if s.Name == name {
			return true
		}
	}
	return false
}

func registryNames() []string {
	out := make([]string, 0, len(cloud.Registry))
	for _, s := range cloud.Registry {
		out = append(out, s.Name)
	}
	return out
}
