package staticfiles

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
)

// exportRoot writes the two shapes a static-site generator emits for a route
// that is not the root: "pricing.html" (the default) and "docs/index.html"
// (trailingSlash). "blog" is both at once — a page with children — which is what
// a section index looks like on disk.
func exportRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	for _, sub := range []string{"docs", "blog", "_next/static/chunks"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	files := map[string]string{
		"index.html":                  "<!doctype html><title>home</title>",
		"pricing.html":                "<!doctype html><title>pricing</title>",
		"docs/index.html":             "<!doctype html><title>docs</title>",
		"blog.html":                   "<!doctype html><title>blog</title>",
		"blog/first-post.html":        "<!doctype html><title>first</title>",
		"_next/static/chunks/main.js": "console.log('main')\n",
		"404.html":                    "<!doctype html><title>gone</title>",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// get returns status, the Location header, and the body.
func get(t *testing.T, srv *httptest.Server, path string) (int, string, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	// No redirect following: a redirect is itself a result under test.
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Location"), string(body)
}

// getFollow returns the final status and body after redirects, which is what a
// browser experiences.
func getFollow(t *testing.T, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// TestStaticExportRoutesResolve is the regression this plane was missing: a
// prerendered site has no router, so "/pricing" is a FILE, and serving only the
// literal key 404s every route but the root. hanzo.ai (772 routes) answered 404
// on all of them from an origin holding the complete site.
//
// Asserted after redirects because the two origins canonicalize differently and
// both are correct: on a local directory "/docs" is a real dir, so it 301s to
// "/docs/" the way every static server does; on the object store there are no
// directories, so the ladder resolves "docs/index.html" in one hop. What must be
// identical is what the reader ends up with.
func TestStaticExportRoutesResolve(t *testing.T) {
	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: exportRoot(t)}))
	defer srv.Close()

	for _, tc := range []struct {
		path string
		want string
	}{
		{"/pricing", "<title>pricing</title>"},       // pricing.html
		{"/pricing/", "<title>pricing</title>"},      // trailing slash, same page
		{"/docs", "<title>docs</title>"},             // docs/index.html
		{"/docs/", "<title>docs</title>"},            // directory index
		{"/blog", "<title>blog</title>"},             // page that also has children
		{"/blog/", "<title>blog</title>"},            // dir with no index, blog.html
		{"/blog/first-post", "<title>first</title>"}, // nested route
	} {
		status, body := getFollow(t, srv, tc.path)
		if status != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 — a prerendered route the origin holds", tc.path, status)
			continue
		}
		if !strings.Contains(body, tc.want) {
			t.Errorf("GET %s served the wrong page: want %q in body, got %q", tc.path, tc.want, truncate(body))
		}
	}
}

// TestFileBackedRouteDoesNotRedirect pins the no-directory case — the exact shape
// a Next.js export writes and the one the object store presents for every route.
// It must resolve in ONE hop, with no Location bouncing the reader through a URL
// that is not the one they asked for.
func TestFileBackedRouteDoesNotRedirect(t *testing.T) {
	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: exportRoot(t)}))
	defer srv.Close()

	status, location, body := get(t, srv, "/pricing")
	if status != http.StatusOK {
		t.Fatalf("GET /pricing = %d (Location: %q), want 200 straight from pricing.html", status, location)
	}
	if location != "" {
		t.Errorf("GET /pricing sent Location: %q — a file-backed route needs no redirect", location)
	}
	if !strings.Contains(body, "<title>pricing</title>") {
		t.Errorf("GET /pricing body = %q", truncate(body))
	}
}

// TestRootServesIndexWithoutRedirect pins that "/" is answered, not moved. The
// index loop used to 301 to "/index.html", which relocates every site's
// canonical URL onto a path nothing links to.
func TestRootServesIndexWithoutRedirect(t *testing.T) {
	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: exportRoot(t)}))
	defer srv.Close()

	status, location, body := get(t, srv, "/")
	if status != http.StatusOK {
		t.Fatalf("GET / = %d (Location: %q), want 200 serving index.html", status, location)
	}
	if location != "" {
		t.Errorf("GET / sent Location: %q — the root must be served, not redirected", location)
	}
	if !strings.Contains(body, "<title>home</title>") {
		t.Errorf("GET / body = %q, want the index", truncate(body))
	}
}

// TestMissingAssetStaysBare is the guard on the fix: the candidate ladder must
// never answer a missing content-hashed chunk with a page. A chunk answered by
// HTML is how a stale asset graph turns into an unreadable ChunkLoadError.
func TestMissingAssetStaysBare(t *testing.T) {
	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: exportRoot(t)}))
	defer srv.Close()

	for _, p := range []string{
		"/_next/static/chunks/nope.js",
		"/_next/static/chunks/nope.css",
		"/missing.woff2",
	} {
		status, _, body := get(t, srv, p)
		if status != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, status)
		}
		if strings.Contains(body, "<title>") {
			t.Errorf("GET %s answered with a page (%q) — an asset miss must stay bare", p, truncate(body))
		}
	}
}

// TestUnknownRouteIsNotFound proves the ladder does not invent pages: a route
// with no file behind it is still a 404, not somebody else's page.
func TestUnknownRouteIsNotFound(t *testing.T) {
	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: exportRoot(t)}))
	defer srv.Close()

	if status, _, _ := get(t, srv, "/no-such-route"); status != http.StatusNotFound {
		t.Errorf("GET /no-such-route = %d, want 404", status)
	}
}

// TestSPAModeStillFallsBack proves the SPA plane is untouched: an extensionless
// miss still reaches the shell, and an asset miss still does not.
func TestSPAModeStillFallsBack(t *testing.T) {
	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: exportRoot(t), SPAMode: true}))
	defer srv.Close()

	status, _, body := get(t, srv, "/client/route")
	if status != http.StatusOK || !strings.Contains(body, "<title>home</title>") {
		t.Errorf("SPA route = %d body=%q, want 200 shell", status, truncate(body))
	}

	if status, _, _ := get(t, srv, "/_next/static/chunks/nope.js"); status != http.StatusNotFound {
		t.Errorf("SPA asset miss = %d, want 404", status)
	}
}

func truncate(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}
