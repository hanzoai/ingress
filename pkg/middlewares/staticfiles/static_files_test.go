package staticfiles

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
)

// localRoot writes a small site to a temp dir and returns the path.
func localRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"index.html":    "<!doctype html><title>home</title>",
		"assets/app.js": "export const v = 1;\n",
		"404.html":      "local not found",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func newLocal(t *testing.T, cfg dynamic.StaticFiles) http.Handler {
	t.Helper()
	h, err := New(context.Background(), http.NotFoundHandler(), cfg, "local")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h
}

// TestLocalOriginUnchanged proves the local-directory origin still serves files,
// resolves the root index, and derives content-type — unaffected by the S3 work.
func TestLocalOriginUnchanged(t *testing.T) {
	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: localRoot(t)}))
	defer srv.Close()

	// Direct asset.
	resp, err := srv.Client().Get(srv.URL + "/assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("export const v")) {
		t.Fatalf("local asset: status=%d body=%q", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("local asset content-type = %q", ct)
	}

	// Root resolves to index.html (via directory redirect, as it always has).
	resp2, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK || !bytes.Contains(body2, []byte("<title>home</title>")) {
		t.Fatalf("local root: status=%d body=%q", resp2.StatusCode, body2)
	}
}

// TestLocalSPAFallback proves the SPA fallback still works after serveFile was
// rerouted through the http.FileSystem (was os.Open).
func TestLocalSPAFallback(t *testing.T) {
	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: localRoot(t), SPAMode: true}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/deep/client/route")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("<title>home</title>")) {
		t.Fatalf("local spa fallback: status=%d body=%q", resp.StatusCode, body)
	}
}

// TestLocalErrorPage proves the custom 404 page still works after the refactor.
func TestLocalErrorPage(t *testing.T) {
	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: localRoot(t), ErrorPage404: "404.html"}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/nope")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(body, []byte("local not found")) {
		t.Fatalf("local error page: status=%d body=%q", resp.StatusCode, body)
	}
}

// TestNosniffTypesAreRegistered proves every extension this plane serves gets a
// Content-Type. The pairing is what makes it matter: the handler sets
// `X-Content-Type-Options: nosniff`, so a missing type is not a browser falling
// back to a guess, it is a browser refusing to render. Go's built-in table stops
// at the web's first dozen extensions and the rest come from the system
// mime.types, which the runtime image does not ship — so this passed on a
// developer's box and failed in production until the types were registered here.
func TestNosniffTypesAreRegistered(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"readme.md":  "# hello\n",
		"font.woff2": "\x77OF2",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: dir}))
	defer srv.Close()

	for path, want := range map[string]string{
		"/readme.md":  "text/markdown",
		"/font.woff2": "font/woff2",
	} {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, want) {
			t.Errorf("%s: Content-Type %q, want %s — with nosniff the browser will not render this", path, got, want)
		}
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: nosniff missing; this test is only meaningful beside it", path)
		}
	}
}
