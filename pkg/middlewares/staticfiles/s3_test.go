package staticfiles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
)

// mapStore is an in-memory objectStore for hermetic tests. It exercises the real
// s3FS and the real handler; only the transport is a double.
type mapStore struct {
	objects map[string][]byte
	mod     time.Time
}

func newMapStore(objs map[string][]byte) *mapStore {
	return &mapStore{objects: objs, mod: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
}

func etagOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:16])
}

func (m *mapStore) stat(_ context.Context, key string) (objectInfo, error) {
	b, ok := m.objects[key]
	if !ok {
		return objectInfo{}, fs.ErrNotExist
	}
	return objectInfo{key: key, size: int64(len(b)), modTime: m.mod, etag: etagOf(b)}, nil
}

func (m *mapStore) open(_ context.Context, key string) (readSeekCloser, error) {
	b, ok := m.objects[key]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return nopSeekCloser{bytes.NewReader(b)}, nil
}

func (m *mapStore) list(_ context.Context, prefix string) ([]objectInfo, error) {
	seen := map[string]bool{}
	var out []objectInfo
	for k, b := range m.objects {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			dir := prefix + rest[:i]
			if !seen[dir] {
				seen[dir] = true
				out = append(out, objectInfo{key: dir, isDir: true, modTime: m.mod})
			}
			continue
		}
		out = append(out, objectInfo{key: k, size: int64(len(b)), modTime: m.mod, etag: etagOf(b)})
	}
	return out, nil
}

type nopSeekCloser struct{ *bytes.Reader }

func (nopSeekCloser) Close() error { return nil }

// s3Handler builds the real middleware over the injected store, applying the
// same defaults as New so tests track production behavior.
func s3Handler(store objectStore, prefix string, cfg dynamic.StaticFiles) *staticFiles {
	indexFiles := cfg.IndexFiles
	if len(indexFiles) == 0 {
		indexFiles = []string{"index.html", "index.htm"}
	}
	spaIndex := cfg.SPAIndex
	if spaIndex == "" {
		spaIndex = "index.html"
	}
	code := http.StatusNotFound
	if cfg.ErrorPage404 != "" {
		code = http.StatusOK
	}
	return &staticFiles{
		root:                 &s3FS{store: store, prefix: prefix},
		enableDirListing:     cfg.EnableDirectoryListing,
		indexFiles:           indexFiles,
		spaMode:              cfg.SPAMode,
		spaIndex:             spaIndex,
		errorPage404:         cfg.ErrorPage404,
		cacheControl:         cfg.CacheControl,
		notFoundResponseCode: code,
		name:                 "test",
		next:                 http.NotFoundHandler(),
	}
}

func TestS3IndexServedAtRoot(t *testing.T) {
	store := newMapStore(map[string][]byte{
		"cd/index.html": []byte("<!doctype html><title>cd</title>"),
	})
	srv := httptest.NewServer(s3Handler(store, "cd", dynamic.StaticFiles{}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("root: want 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "<title>cd</title>") {
		t.Fatalf("root did not resolve to index.html, got %q", body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("root content-type = %q, want text/html", ct)
	}
}

func TestS3AssetContentTypeAndValidators(t *testing.T) {
	js := []byte("export const x = 1;\n")
	store := newMapStore(map[string][]byte{
		"cd/assets/app.js": js,
	})
	srv := httptest.NewServer(s3Handler(store, "cd", dynamic.StaticFiles{}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("asset: want 200, got %d", resp.StatusCode)
	}
	if !bytes.Equal(body, js) {
		t.Fatalf("asset body mismatch: got %q", body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("asset content-type = %q, want javascript (derived from extension)", ct)
	}
	if resp.Header.Get("Cache-Control") == "" {
		t.Fatal("asset missing Cache-Control")
	}
	wantETag := `"` + etagOf(js) + `"`
	if got := resp.Header.Get("ETag"); got != wantETag {
		t.Fatalf("asset ETag = %q, want %q", got, wantETag)
	}
}

func TestS3ConditionalRequestNotModified(t *testing.T) {
	js := []byte("console.log('hi')\n")
	store := newMapStore(map[string][]byte{"cd/a.js": js})
	srv := httptest.NewServer(s3Handler(store, "cd", dynamic.StaticFiles{}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/a.js", nil)
	req.Header.Set("If-None-Match", `"`+etagOf(js)+`"`)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("If-None-Match: want 304, got %d", resp.StatusCode)
	}
}

func TestS3SPAFallback(t *testing.T) {
	index := []byte("<!doctype html><div id=app></div>")
	store := newMapStore(map[string][]byte{"cd/index.html": index})
	srv := httptest.NewServer(s3Handler(store, "cd", dynamic.StaticFiles{SPAMode: true}))
	defer srv.Close()

	// A deep client route with no backing object must serve index.html (200),
	// not 404 — that is the whole point of SPA mode.
	resp, err := srv.Client().Get(srv.URL + "/applications/deploy/123")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("spa deep route: want 200, got %d", resp.StatusCode)
	}
	if !bytes.Equal(body, index) {
		t.Fatalf("spa deep route did not serve index.html, got %q", body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("spa fallback content-type = %q", ct)
	}
}

func TestS3MissingAssetNonSPA404(t *testing.T) {
	store := newMapStore(map[string][]byte{"cd/index.html": []byte("x")})
	srv := httptest.NewServer(s3Handler(store, "cd", dynamic.StaticFiles{})) // non-spa, no error page
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/missing.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing asset non-spa: want 404, got %d", resp.StatusCode)
	}
}

func TestS3ErrorPage(t *testing.T) {
	store := newMapStore(map[string][]byte{
		"cd/index.html": []byte("home"),
		"cd/404.html":   []byte("custom not found"),
	})
	srv := httptest.NewServer(s3Handler(store, "cd", dynamic.StaticFiles{ErrorPage404: "404.html"}))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/does/not/exist")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "custom not found") {
		t.Fatalf("error page not served, got %q", body)
	}
}

func TestS3RangeRequestStreams(t *testing.T) {
	// A body large enough that a Range read is a genuine partial fetch.
	payload := bytes.Repeat([]byte("0123456789abcdef"), 4096) // 64 KiB
	store := newMapStore(map[string][]byte{"cd/big.bin": payload})
	srv := httptest.NewServer(s3Handler(store, "cd", dynamic.StaticFiles{}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/big.bin", nil)
	req.Header.Set("Range", "bytes=100-199")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("range: want 206, got %d", resp.StatusCode)
	}
	if len(body) != 100 {
		t.Fatalf("range length = %d, want 100", len(body))
	}
	if !bytes.Equal(body, payload[100:200]) {
		t.Fatal("range bytes mismatch")
	}
}

func TestS3CacheControlByExtension(t *testing.T) {
	store := newMapStore(map[string][]byte{"cd/app.js": []byte("x")})
	cfg := dynamic.StaticFiles{CacheControl: map[string]string{".js": "max-age=31536000, immutable"}}
	srv := httptest.NewServer(s3Handler(store, "cd", cfg))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/app.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Cache-Control"); got != "max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

// TestS3PrefixIsolationTraversal proves a request can never read outside its own
// site prefix, even with ../ escapes. Uses a raw recorder so no client/server
// path-cleaning masks the check.
func TestS3PrefixIsolationTraversal(t *testing.T) {
	store := newMapStore(map[string][]byte{
		"cd/index.html":     []byte("cd home"),
		"other/secret.html": []byte("SECRET"), // a different site's prefix
		"secret.txt":        []byte("ROOT SECRET"),
	})
	h := s3Handler(store, "cd", dynamic.StaticFiles{})

	for _, target := range []string{
		"/../other/secret.html",
		"/../../other/secret.html",
		"/../secret.txt",
		"/a/../../secret.txt",
		"/%2e%2e/other/secret.html",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://x"+target, nil)
		h.ServeHTTP(rec, req)
		body := rec.Body.String()
		if strings.Contains(body, "SECRET") || strings.Contains(body, "ROOT SECRET") {
			t.Fatalf("traversal %q leaked cross-prefix content: %q (status %d)", target, body, rec.Code)
		}
	}
}

// TestKeyForNeverEscapesPrefix is the deterministic unit-level proof of the
// traversal defense.
func TestKeyForNeverEscapesPrefix(t *testing.T) {
	s := &s3FS{prefix: "cd"}
	cases := map[string]string{
		"/index.html":       "cd/index.html",
		"/assets/app.js":    "cd/assets/app.js",
		"/../secret":        "cd/secret",
		"/../../etc/passwd": "cd/etc/passwd",
		"/a/../../b":        "cd/b",
		"/./x":              "cd/x",
		"//x":               "cd/x",
		"/":                 "cd",
	}
	for in, want := range cases {
		if got := s.keyFor(in); got != want {
			t.Errorf("keyFor(%q) = %q, want %q", in, got, want)
		}
		if got := s.keyFor(in); !strings.HasPrefix(got, "cd") {
			t.Errorf("keyFor(%q) = %q escaped prefix", in, got)
		}
	}
}

func TestParseObjectRoot(t *testing.T) {
	cases := []struct {
		root   string
		bucket string
		prefix string
		ok     bool
	}{
		{"s3://cdn/cd", "cdn", "cd", true},
		{"s3://cdn/cd/", "cdn", "cd", true},
		{"s3://cdn/sites/cd/", "cdn", "sites/cd", true},
		{"s3://cdn", "cdn", "", true},
		{"s3://cdn/", "cdn", "", true},
		{"/var/www", "", "", false},
		{".", "", "", false},
		{"s3://", "", "", false},
		{"http://cdn/cd", "", "", false},
	}
	for _, c := range cases {
		b, p, ok := parseObjectRoot(c.root)
		if ok != c.ok || b != c.bucket || p != c.prefix {
			t.Errorf("parseObjectRoot(%q) = (%q,%q,%v), want (%q,%q,%v)", c.root, b, p, ok, c.bucket, c.prefix, c.ok)
		}
	}
}

// TestNewObjectFSFailsClosed proves the middleware refuses to build an S3 origin
// without an endpoint or credentials, rather than silently serving nothing.
func TestNewObjectFSFailsClosed(t *testing.T) {
	// No endpoint anywhere.
	t.Setenv("S3_ENDPOINT", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	if _, err := newObjectFS("s3://cdn/cd", "", ""); err == nil {
		t.Fatal("expected error with no endpoint")
	}

	// Endpoint present, credentials absent.
	if _, err := newObjectFS("s3://cdn/cd", "s3.local:9000", ""); err == nil {
		t.Fatal("expected error with no credentials")
	}

	// Endpoint + credentials present → builds.
	t.Setenv("AWS_ACCESS_KEY_ID", "ak")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "sk")
	if _, err := newObjectFS("s3://cdn/cd", "s3.local:9000", ""); err != nil {
		t.Fatalf("expected success with endpoint+creds, got %v", err)
	}
}
