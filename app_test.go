// Copyright © 2026 Hanzo AI. MIT License.

package ingress

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApp_ServesHealthAndConfig(t *testing.T) {
	t.Setenv("INGRESS_ENTRYPOINTS", "web,websecure")
	t.Setenv("INGRESS_PROVIDERS", "kubernetes,file")
	t.Setenv("INGRESS_ACME_ENABLED", "true")

	app, err := App("hanzo", "api.hanzo.ai")
	if err != nil {
		t.Fatalf("App: %v", err)
	}

	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/_/ingress/healthz", []string{`"service":"ingress"`}},
		{"/_/ingress/config", []string{
			`"brand":"hanzo"`,
			`"domain":"api.hanzo.ai"`,
			`"entrypoints":["web","websecure"]`,
			`"providers":["kubernetes","file"]`,
			`"acme_enabled":true`,
		}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			resp, err := app.Fiber().Test(httptest.NewRequest("GET", tc.path, nil))
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			if resp.StatusCode != 200 {
				t.Fatalf("status: got %d want 200", resp.StatusCode)
			}
			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(string(raw), want) {
					t.Fatalf("body: got %q, want substring %q", string(raw), want)
				}
			}
		})
	}
}
