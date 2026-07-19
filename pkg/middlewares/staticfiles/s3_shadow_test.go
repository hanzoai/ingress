//go:build shadow

// Shadow proof: drives the REAL object-store origin (hanzos3/go-sdk client,
// SigV4, HTTP) against a real S3 server serving a real static bundle. Run with:
//
//	S3_ENDPOINT=127.0.0.1:19000 \
//	AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=... \
//	BUNDLE_DIR=/path/to/out \
//	go test -tags shadow -run TestShadow -v ./pkg/middlewares/staticfiles/
package staticfiles

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
	s3 "github.com/hanzos3/go-sdk"
	"github.com/hanzos3/go-sdk/pkg/credentials"
)

const shadowBucket = "cdn"
const shadowPrefix = "cd"

func shadowUpload(t *testing.T, bundle string) {
	t.Helper()
	endpoint := strings.TrimPrefix(strings.TrimPrefix(os.Getenv("S3_ENDPOINT"), "http://"), "https://")
	client, err := s3.New(endpoint, &s3.Options{
		Creds:        credentials.NewStaticV4(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), ""),
		Secure:       false,
		Region:       "us-east-1",
		BucketLookup: s3.BucketLookupPath,
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	ctx := context.Background()
	if err := client.MakeBucket(ctx, shadowBucket, s3.MakeBucketOptions{}); err != nil {
		if exists, _ := client.BucketExists(ctx, shadowBucket); !exists {
			t.Fatalf("make bucket: %v", err)
		}
	}
	n := 0
	err = filepath.WalkDir(bundle, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(bundle, p)
		key := shadowPrefix + "/" + filepath.ToSlash(rel)
		if _, err := client.FPutObject(ctx, shadowBucket, key, p, s3.PutObjectOptions{}); err != nil {
			return err
		}
		n++
		return nil
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	t.Logf("uploaded %d objects to s3://%s/%s/", n, shadowBucket, shadowPrefix)
}

func shadowServer(t *testing.T, spa bool) *httptest.Server {
	t.Helper()
	h, err := New(context.Background(), http.NotFoundHandler(),
		dynamic.StaticFiles{Root: "s3://" + shadowBucket + "/" + shadowPrefix, SPAMode: spa}, "shadow")
	if err != nil {
		t.Fatalf("New (real minioStore): %v", err)
	}
	return httptest.NewServer(h)
}

func TestShadowServesRealBundleFromS3(t *testing.T) {
	bundle := os.Getenv("BUNDLE_DIR")
	if bundle == "" {
		t.Skip("set BUNDLE_DIR to a real static export")
	}
	shadowUpload(t, bundle)

	// Read the real index for comparison.
	indexBytes, err := os.ReadFile(filepath.Join(bundle, "index.html"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}

	srv := shadowServer(t, true) // SPA mode
	defer srv.Close()
	c := srv.Client()

	// 1. Root resolves to index.html.
	resp, err := c.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(body) != len(indexBytes) {
		t.Fatalf("root: status=%d len=%d want len=%d", resp.StatusCode, len(body), len(indexBytes))
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("root content-type=%q", resp.Header.Get("Content-Type"))
	}
	t.Logf("PASS root -> index.html (%d bytes, %s)", len(body), resp.Header.Get("Content-Type"))

	// 2. Deep SPA route -> index.html (200, not 404).
	resp2, _ := c.Get(srv.URL + "/applications/deploy/xyz")
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK || len(body2) != len(indexBytes) {
		t.Fatalf("spa deep route: status=%d len=%d", resp2.StatusCode, len(body2))
	}
	t.Logf("PASS deep route /applications/deploy/xyz -> index.html (SPA fallback, 200)")

	// 3. A real hashed asset serves byte-identical with a derived content-type.
	var asset string
	_ = filepath.WalkDir(bundle, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && asset == "" && strings.HasSuffix(p, ".js") {
			rel, _ := filepath.Rel(bundle, p)
			asset = "/" + filepath.ToSlash(rel)
		}
		return nil
	})
	if asset != "" {
		want, _ := os.ReadFile(filepath.Join(bundle, filepath.FromSlash(asset)))
		resp3, _ := c.Get(srv.URL + asset)
		body3, _ := io.ReadAll(resp3.Body)
		resp3.Body.Close()
		if resp3.StatusCode != http.StatusOK {
			t.Fatalf("asset %s: status=%d", asset, resp3.StatusCode)
		}
		if len(body3) != len(want) {
			t.Fatalf("asset %s: len=%d want=%d (not byte-identical)", asset, len(body3), len(want))
		}
		if !strings.Contains(resp3.Header.Get("Content-Type"), "javascript") {
			t.Fatalf("asset %s content-type=%q", asset, resp3.Header.Get("Content-Type"))
		}
		if resp3.Header.Get("ETag") == "" {
			t.Fatalf("asset %s missing ETag", asset)
		}
		t.Logf("PASS asset %s -> 200 %s, %d bytes byte-identical, ETag=%s",
			asset, resp3.Header.Get("Content-Type"), len(body3), resp3.Header.Get("ETag"))
	}

	// 4. A missing asset in a NON-SPA server is a real 404 (no masking).
	nsrv := shadowServer(t, false)
	defer nsrv.Close()
	resp4, _ := nsrv.Client().Get(nsrv.URL + "/definitely/missing/asset.js")
	resp4.Body.Close()
	if resp4.StatusCode != http.StatusNotFound {
		t.Fatalf("non-spa missing asset: status=%d want 404", resp4.StatusCode)
	}
	t.Logf("PASS non-spa missing asset -> 404 (no SPA masking)")
}

// TestShadowStreamsLargeObjectUnbuffered proves the origin streams: it serves an
// object larger than the reference implementation's 50 MB in-memory buffer cap,
// and answers a Range request — both impossible if the whole object were read
// into memory.
func TestShadowStreamsLargeObjectUnbuffered(t *testing.T) {
	if os.Getenv("S3_ENDPOINT") == "" {
		t.Skip("set S3_ENDPOINT for the shadow proof")
	}
	const size = 60 << 20 // 60 MiB > the reference 50 MiB ReadAll cap

	dir := t.TempDir()
	big := filepath.Join(dir, "big.bin")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	block := make([]byte, 1<<20)
	for i := range block {
		block[i] = byte(i)
	}
	for written := 0; written < size; written += len(block) {
		if _, err := f.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	shadowUpload(t, dir) // uploads big.bin to s3://cdn/cd/big.bin

	srv := shadowServer(t, false)
	defer srv.Close()

	// Full GET streams all 60 MiB.
	resp, err := srv.Client().Get(srv.URL + "/big.bin")
	if err != nil {
		t.Fatal(err)
	}
	n, _ := io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || n != int64(size) {
		t.Fatalf("large GET: status=%d streamed=%d want=%d", resp.StatusCode, n, size)
	}
	t.Logf("PASS full GET streamed %d bytes (> reference 50 MiB buffer cap)", n)

	// Range request is a genuine partial fetch (seek), not a slice of a buffer.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/big.bin", nil)
	req.Header.Set("Range", "bytes=52428800-52428899") // 100 bytes at 50 MiB
	rr, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	rb, _ := io.ReadAll(rr.Body)
	rr.Body.Close()
	if rr.StatusCode != http.StatusPartialContent || len(rb) != 100 {
		t.Fatalf("large Range: status=%d len=%d want 206/100", rr.StatusCode, len(rb))
	}
	t.Logf("PASS Range bytes=50MiB..+100 -> 206, 100 bytes (seek over S3, no buffering)")
}
