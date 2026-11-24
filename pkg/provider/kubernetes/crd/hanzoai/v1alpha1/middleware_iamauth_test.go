package v1alpha1

import (
	"testing"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
)

func TestMiddlewareSpecIAMAuthDeepCopy(t *testing.T) {
	redir := true
	in := &MiddlewareSpec{
		IAMAuth: &dynamic.IAMAuth{
			Issuer:          "https://hanzo.id",
			ClientID:        "hanzo-flow",
			CookieName:      "hanzo_iam_session",
			SkipPaths:       []string{"/healthz", "/public/"},
			BrowserRedirect: &redir,
		},
	}

	out := in.DeepCopy()
	if out.IAMAuth == nil {
		t.Fatal("IAMAuth dropped by DeepCopy")
	}
	if out.IAMAuth == in.IAMAuth {
		t.Fatal("IAMAuth not deep-copied (shared pointer)")
	}
	if out.IAMAuth.ClientID != "hanzo-flow" || out.IAMAuth.BrowserRedirect == nil || !*out.IAMAuth.BrowserRedirect {
		t.Fatalf("IAMAuth values not copied: %+v", out.IAMAuth)
	}

	out.IAMAuth.SkipPaths[0] = "/mutated"
	*out.IAMAuth.BrowserRedirect = false
	if in.IAMAuth.SkipPaths[0] != "/healthz" {
		t.Fatal("SkipPaths slice aliased between original and copy")
	}
	if !*in.IAMAuth.BrowserRedirect {
		t.Fatal("BrowserRedirect pointer aliased between original and copy")
	}
}
