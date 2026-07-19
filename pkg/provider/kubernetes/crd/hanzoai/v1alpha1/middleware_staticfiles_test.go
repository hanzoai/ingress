package v1alpha1

import (
	"testing"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
)

// TestMiddlewareSpecStaticFilesDeepCopy proves the CRD Middleware spec carries
// staticFiles (with the S3-origin fields) and deep-copies it independently, so
// a staticFiles Middleware CR survives the informer cache round-trip instead of
// being silently dropped.
func TestMiddlewareSpecStaticFilesDeepCopy(t *testing.T) {
	in := &MiddlewareSpec{
		StaticFiles: &dynamic.StaticFiles{
			Root:         "s3://cdn/cd",
			Endpoint:     "s3.hanzo.svc.cluster.local:9000",
			Region:       "us-east-1",
			SPAMode:      true,
			SPAIndex:     "index.html",
			IndexFiles:   []string{"index.html"},
			CacheControl: map[string]string{".js": "max-age=31536000, immutable"},
		},
	}

	out := in.DeepCopy()

	if out.StaticFiles == nil {
		t.Fatal("StaticFiles dropped by DeepCopy")
	}
	if out.StaticFiles == in.StaticFiles {
		t.Fatal("StaticFiles not deep-copied (shared pointer)")
	}
	if out.StaticFiles.Root != "s3://cdn/cd" || out.StaticFiles.Endpoint != "s3.hanzo.svc.cluster.local:9000" || !out.StaticFiles.SPAMode {
		t.Fatalf("StaticFiles values not copied: %+v", out.StaticFiles)
	}

	// A mutation of the copy must not reach the original — the map/slice were
	// deep-copied, not aliased.
	out.StaticFiles.CacheControl[".js"] = "changed"
	if in.StaticFiles.CacheControl[".js"] == "changed" {
		t.Fatal("CacheControl map is aliased between original and copy")
	}
}
