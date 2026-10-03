package staticfiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/hanzoai/ingress/pkg/config/dynamic"
)

// liveStore is an objectStore whose objects a test can overwrite while it serves,
// counting what the cache asks of it.
type liveStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	mods    map[string]time.Time
	down    bool
	gate    chan struct{}     // when set, stat waits on it
	lag     map[string][]byte // what the next read of a key returns instead, once
	fail    map[string]int    // reads of a key that fail before one succeeds

	stats, opens atomic.Int64
}

func newLiveStore(objs map[string][]byte) *liveStore {
	s := &liveStore{objects: map[string][]byte{}, mods: map[string]time.Time{}, lag: map[string][]byte{}, fail: map[string]int{}}
	for k, v := range objs {
		s.publish(k, v)
	}
	return s
}

// publish writes key in place, the way a Sites deploy does.
func (s *liveStore) publish(key string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = body
	s.mods[key] = time.Date(2026, 1, 2, 3, 4, 5, len(s.mods)*1000, time.UTC)
}

func (s *liveStore) stat(_ context.Context, key string) (objectInfo, error) {
	s.stats.Add(1)
	if s.gate != nil {
		<-s.gate
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return objectInfo{}, fmt.Errorf("%w: refused", errObjectStoreUnavailable)
	}
	b, ok := s.objects[key]
	if !ok {
		return objectInfo{}, fs.ErrNotExist
	}
	return objectInfo{key: key, size: int64(len(b)), modTime: s.mods[key], etag: etagOf(b)}, nil
}

// open honors If-Match the way S3 does: a read whose version is not etag fails
// with 412, even when the bytes come from a copy one write behind.
func (s *liveStore) open(_ context.Context, key, etag string) (readSeekCloser, error) {
	s.opens.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail[key] > 0 {
		s.fail[key]--
		return nil, fmt.Errorf("%w: reset by peer", errObjectStoreUnavailable)
	}
	b, ok := s.objects[key]
	if old, behind := s.lag[key]; behind {
		delete(s.lag, key)
		b, ok = old, true
	}
	if !ok {
		return nil, fs.ErrNotExist
	}
	if etag != "" && etagOf(b) != etag {
		return nil, errors.New("412 PreconditionFailed")
	}
	return nopSeekCloser{bytes.NewReader(b)}, nil
}

func (s *liveStore) list(context.Context, string) ([]objectInfo, error) { return nil, nil }

// clock is a settable now for the cache.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func cachedServer(t *testing.T, store objectStore, c *objectCache, cfg dynamic.StaticFiles) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(s3Handler(&cachedStore{objectStore: store, bucket: "sites", cache: c}, "org/site", cfg))
	t.Cleanup(srv.Close)
	return srv
}

func testCache(limit, max int64) (*objectCache, *clock) {
	clk := &clock{t: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)}
	c := newObjectCache(limit, 1<<20, max, storeConns, time.Second)
	c.now = clk.now
	return c, clk
}

func body(t *testing.T, srv *httptest.Server, path string, header ...string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	// The transport would otherwise ask for gzip and hide Content-Encoding.
	tr := &http.Transport{DisableCompression: true}
	defer tr.CloseIdleConnections()
	resp, err := (&http.Client{Transport: tr}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// TestCacheServesFreshFromMemory: within fresh a repeat request asks the store
// nothing, and the response is the same as the first.
func TestCacheServesFreshFromMemory(t *testing.T) {
	store := newLiveStore(map[string][]byte{"org/site/index.html": []byte("<title>v1</title>")})
	c, _ := testCache(1<<20, 1<<20)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})

	for range 3 {
		if resp, got := body(t, srv, "/"); resp.StatusCode != 200 || got != "<title>v1</title>" {
			t.Fatalf("GET / = %d %q", resp.StatusCode, got)
		}
	}
	if s, o := store.stats.Load(), store.opens.Load(); s != 1 || o != 1 {
		t.Fatalf("store asked %d stats, %d opens for three requests within fresh, want 1 and 1", s, o)
	}
}

// TestCacheRevalidatesWithOneHead: after fresh, an unchanged object costs one
// stat and no read.
func TestCacheRevalidatesWithOneHead(t *testing.T) {
	store := newLiveStore(map[string][]byte{"org/site/app.js": []byte("main()")})
	c, clk := testCache(1<<20, 1<<20)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})

	body(t, srv, "/app.js")
	clk.advance(2 * time.Second)
	if _, got := body(t, srv, "/app.js"); got != "main()" {
		t.Fatalf("revalidated body = %q", got)
	}
	if s, o := store.stats.Load(), store.opens.Load(); s != 2 || o != 1 {
		t.Fatalf("store asked %d stats, %d opens, want 2 and 1: an unchanged ETag keeps the bytes", s, o)
	}
}

// TestCachePublishShowsAfterFresh: a deploy overwrites keys in place. The old
// bytes serve for at most fresh, then the new ones, with the new validator.
func TestCachePublishShowsAfterFresh(t *testing.T) {
	store := newLiveStore(map[string][]byte{"org/site/index.html": []byte("<title>v1</title>")})
	c, clk := testCache(1<<20, 1<<20)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})

	first, _ := body(t, srv, "/")
	store.publish("org/site/index.html", []byte("<title>v2</title>"))

	if _, got := body(t, srv, "/"); got != "<title>v1</title>" {
		t.Fatalf("within fresh = %q, want the held v1", got)
	}
	clk.advance(time.Second)
	resp, got := body(t, srv, "/")
	if got != "<title>v2</title>" {
		t.Fatalf("after fresh = %q, want the published v2", got)
	}
	if resp.Header.Get("ETag") == first.Header.Get("ETag") {
		t.Fatalf("ETag %q did not move with the publish", resp.Header.Get("ETag"))
	}
}

// TestCacheHoldsAbsence: the page ladder's probe for the bare key is held like
// an object, and a page published under a held miss appears after fresh.
func TestCacheHoldsAbsence(t *testing.T) {
	store := newLiveStore(map[string][]byte{"org/site/pricing.html": []byte("<title>pricing</title>")})
	c, clk := testCache(1<<20, 1<<20)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})

	body(t, srv, "/pricing")
	stats := store.stats.Load()
	if resp, got := body(t, srv, "/pricing"); resp.StatusCode != 200 || got != "<title>pricing</title>" {
		t.Fatalf("GET /pricing = %d %q", resp.StatusCode, got)
	}
	if n := store.stats.Load() - stats; n != 0 {
		t.Fatalf("repeat /pricing asked the store %d times, want 0", n)
	}

	if resp, _ := body(t, srv, "/new"); resp.StatusCode != 404 {
		t.Fatalf("GET /new = %d before it is published", resp.StatusCode)
	}
	store.publish("org/site/new.html", []byte("<title>new</title>"))
	clk.advance(time.Second)
	if resp, got := body(t, srv, "/new"); resp.StatusCode != 200 || got != "<title>new</title>" {
		t.Fatalf("GET /new after publish = %d %q", resp.StatusCode, got)
	}
}

// TestCacheOutageIsNotHeld: a failed stat is a 502 now and asked again on the
// very next request, not after fresh.
func TestCacheOutageIsNotHeld(t *testing.T) {
	store := newLiveStore(map[string][]byte{"org/site/a.css": []byte("a{}")})
	c, _ := testCache(1<<20, 1<<20)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})

	store.mu.Lock()
	store.down = true
	store.mu.Unlock()
	if resp, _ := body(t, srv, "/a.css"); resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("outage = %d, want 502", resp.StatusCode)
	}
	store.mu.Lock()
	store.down = false
	store.mu.Unlock()
	if resp, got := body(t, srv, "/a.css"); resp.StatusCode != 200 || got != "a{}" {
		t.Fatalf("after recovery = %d %q, want 200 at once", resp.StatusCode, got)
	}
}

// TestCacheServesHeldThroughOutage: a store that stops answering after an object
// was read leaves that object served, and the store is asked again once per fresh.
func TestCacheServesHeldThroughOutage(t *testing.T) {
	store := newLiveStore(map[string][]byte{"org/site/index.html": []byte("<title>held</title>")})
	c, clk := testCache(1<<20, 1<<20)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})

	body(t, srv, "/")
	store.mu.Lock()
	store.down = true
	store.mu.Unlock()
	for range 2 {
		clk.advance(2 * time.Second)
		if resp, got := body(t, srv, "/"); resp.StatusCode != 200 || got != "<title>held</title>" {
			t.Fatalf("during outage = %d %q, want the held page", resp.StatusCode, got)
		}
	}
	if s := store.stats.Load(); s != 3 {
		t.Fatalf("store asked %d stats, want 3: once per request past fresh", s)
	}
}

// TestCacheLargeObjectStreams: an object over max is never held; every request
// streams it, and its stat is still held.
func TestCacheLargeObjectStreams(t *testing.T) {
	big := bytes.Repeat([]byte("x"), 4096)
	store := newLiveStore(map[string][]byte{"org/site/film.mp4": big})
	c, _ := testCache(1<<20, 1024)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})

	for range 2 {
		if _, got := body(t, srv, "/film.mp4"); got != string(big) {
			t.Fatalf("large object body is %d bytes, want %d", len(got), len(big))
		}
	}
	if s, o := store.stats.Load(), store.opens.Load(); s != 1 || o != 2 {
		t.Fatalf("store asked %d stats, %d opens, want 1 and 2", s, o)
	}
}

// TestCacheBound: the cache never holds more than its limit, drops the least
// recently used first, and keeps what was just read.
func TestCacheBound(t *testing.T) {
	// Every id below is 20 bytes, so the limit holds exactly four entries.
	c, _ := testCache(4*(20+100+entryOverhead), 1<<20)
	for i := range 10 {
		c.put(&cacheEntry{id: fmt.Sprintf("sites/k%02d-padding-xx", i), body: make([]byte, 100)})
		if i == 5 {
			c.get("sites/k00-padding-xx") // gone already: a get of an evicted id is a miss
		}
		c.get("sites/k02-padding-xx")
	}
	if c.bytes[0] > c.limits[0] {
		t.Fatalf("held %d bytes over a limit of %d", c.bytes[0], c.limits[0])
	}
	if len(c.index) != c.lists[0].Len()+c.lists[1].Len() {
		t.Fatalf("index holds %d, lists %d", len(c.index), c.lists[0].Len()+c.lists[1].Len())
	}
	if c.get("sites/k02-padding-xx") == nil {
		t.Fatal("the most recently read entry was evicted")
	}
	if c.get("sites/k09-padding-xx") == nil {
		t.Fatal("the newest entry was evicted")
	}
	for _, gone := range []string{"sites/k00-padding-xx", "sites/k01-padding-xx", "sites/k03-padding-xx"} {
		if c.get(gone) != nil {
			t.Fatalf("%s survived; least recently used goes first", gone)
		}
	}

	// Replacing an entry charges the difference, not both.
	c.put(&cacheEntry{id: "sites/k09-padding-xx", body: make([]byte, 10)})
	var sum int64
	for el := c.lists[0].Front(); el != nil; el = el.Next() {
		sum += el.Value.(*cacheEntry).cost()
	}
	if sum != c.bytes[0] {
		t.Fatalf("accounted %d bytes, entries cost %d", c.bytes[0], sum)
	}
}

// TestCacheCoalesces: many requests for one cold object ask the store once.
func TestCacheCoalesces(t *testing.T) {
	store := newLiveStore(map[string][]byte{"org/site/index.html": []byte("<title>one</title>")})
	store.gate = make(chan struct{})
	c, _ := testCache(1<<20, 1<<20)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})

	const n = 32
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(srv.URL + "/index.html")
			if err != nil {
				errs <- err
				return
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != 200 || string(b) != "<title>one</title>" {
				errs <- fmt.Errorf("%d %q", resp.StatusCode, b)
			}
		}()
	}
	// Let every request reach the cache before the one stat answers.
	deadline := time.Now().Add(5 * time.Second)
	for store.stats.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	close(store.gate)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if s, o := store.stats.Load(), store.opens.Load(); s != 1 || o != 1 {
		t.Fatalf("%d concurrent requests asked %d stats, %d opens, want 1 and 1", n, s, o)
	}
}

// TestCacheWaiterCancels: a request that gives up while the store is slow is
// released, and the refresh it joined still lands for the next one.
func TestCacheWaiterCancels(t *testing.T) {
	store := newLiveStore(map[string][]byte{"org/site/a.js": []byte("a")})
	store.gate = make(chan struct{})
	c, _ := testCache(1<<20, 1<<20)
	cs := &cachedStore{objectStore: store, bucket: "sites", cache: c}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cs.stat(ctx, "org/site/a.js"); !errors.Is(err, errObjectStoreUnavailable) {
		t.Fatalf("cancelled wait = %v, want an availability error", err)
	}
	close(store.gate)
	deadline := time.Now().Add(5 * time.Second)
	for c.get("sites/org/site/a.js") == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if info, err := cs.stat(context.Background(), "org/site/a.js"); err != nil || info.size != 1 {
		t.Fatalf("stat after the refresh = %+v %v", info, err)
	}
}

// TestCacheBrotli: text is sent brotli-encoded to a client that accepts it, as
// its own representation, and as identity to one that does not or that asks for
// a range.
func TestCacheBrotli(t *testing.T) {
	page := []byte("<!doctype html><title>br</title>" + strings.Repeat("<p>hanzo</p>", 400))
	store := newLiveStore(map[string][]byte{
		"org/site/index.html": page,
		"org/site/logo.png":   bytes.Repeat([]byte{0x89}, 4096),
	})
	c, _ := testCache(1<<20, 1<<20)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})

	// The first request is never kept waiting on the encoding: it is answered
	// as identity, and the twin is made behind it.
	if resp, got := body(t, srv, "/", "Accept-Encoding", "br"); resp.Header.Get("Content-Encoding") != "" || got != string(page) {
		t.Fatalf("first request = %q encoded, want identity", resp.Header.Get("Content-Encoding"))
	}
	waitEncoded(t, c, "sites/org/site/index.html")

	resp, enc := body(t, srv, "/", "Accept-Encoding", "gzip, deflate, br")
	if resp.Header.Get("Content-Encoding") != "br" {
		t.Fatalf("Content-Encoding = %q, want br", resp.Header.Get("Content-Encoding"))
	}
	if !strings.Contains(resp.Header.Get("Vary"), "Accept-Encoding") {
		t.Fatalf("Vary = %q", resp.Header.Get("Vary"))
	}
	if got := resp.Header.Get("Content-Length"); got != fmt.Sprint(len(enc)) {
		t.Fatalf("Content-Length = %s for %d encoded bytes", got, len(enc))
	}
	dec, err := io.ReadAll(brotli.NewReader(strings.NewReader(enc)))
	if err != nil || !bytes.Equal(dec, page) {
		t.Fatalf("br body does not decode to the page (%v)", err)
	}
	brTag := resp.Header.Get("ETag")
	if !strings.HasSuffix(brTag, `-br"`) {
		t.Fatalf("br ETag = %q, want its own validator", brTag)
	}

	if resp, _ := body(t, srv, "/", "Accept-Encoding", "br", "If-None-Match", brTag); resp.StatusCode != http.StatusNotModified {
		t.Fatalf("If-None-Match on the br validator = %d, want 304", resp.StatusCode)
	}

	for _, h := range [][]string{
		{"Accept-Encoding", "gzip"},
		{"Accept-Encoding", "br;q=0"},
		{"Accept-Encoding", "br", "Range", "bytes=0-9"},
	} {
		resp, got := body(t, srv, "/", h...)
		if resp.Header.Get("Content-Encoding") != "" {
			t.Fatalf("%v: Content-Encoding = %q, want identity", h, resp.Header.Get("Content-Encoding"))
		}
		if !strings.HasPrefix(string(page), got) || got == "" {
			t.Fatalf("%v: body is not the identity page", h)
		}
		if strings.HasSuffix(resp.Header.Get("ETag"), `-br"`) {
			t.Fatalf("%v: identity carries the br validator", h)
		}
	}

	if resp, _ := body(t, srv, "/logo.png", "Accept-Encoding", "br"); resp.Header.Get("Content-Encoding") != "" || resp.Header.Get("Vary") != "" {
		t.Fatalf("an image was encoded: %q / Vary %q", resp.Header.Get("Content-Encoding"), resp.Header.Get("Vary"))
	}
}

func TestAcceptsBrotli(t *testing.T) {
	for header, want := range map[string]bool{
		"":                     false,
		"gzip":                 false,
		"br":                   true,
		"gzip, br":             true,
		"gzip;q=1.0, BR;q=0.5": true,
		"br;q=0":               false,
		"br; q=0.0, gzip":      false,
		"brotli":               false,
		"br;Q=0":               false,
	} {
		if got := acceptsBrotli(header); got != want {
			t.Errorf("acceptsBrotli(%q) = %v, want %v", header, got, want)
		}
	}
}

// TestNextStaticIsImmutable: everything Next writes under _next/static is named
// by its content, so it is cached for a year whatever its extension; the same
// file elsewhere keeps the default.
func TestNextStaticIsImmutable(t *testing.T) {
	store := newMapStore(map[string][]byte{
		"cd/_next/static/media/hero.4f3a9c21.png": []byte("png"),
		"cd/logo.png":                        []byte("png"),
		"cd/pay/_next/static/chunks/main.js": []byte("js"),
	})
	srv := httptest.NewServer(s3Handler(store, "cd", dynamic.StaticFiles{}))
	defer srv.Close()

	for p, want := range map[string]string{
		"/_next/static/media/hero.4f3a9c21.png": "public, max-age=31536000, immutable",
		"/pay/_next/static/chunks/main.js":      "public, max-age=31536000, immutable",
		"/logo.png":                             "max-age=86400",
	} {
		resp, err := srv.Client().Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if got := resp.Header.Get("Cache-Control"); got != want {
			t.Errorf("%s Cache-Control = %q, want %q", p, got, want)
		}
	}
}

// TestObjectFSSharesClient: every root under one store configuration rides one
// client, and so one connection pool.
func TestObjectFSSharesClient(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "http://s3.test:9000")
	t.Setenv("AWS_ACCESS_KEY_ID", "k")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "s")
	a, err := newObjectFS("s3://sites/org/a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := newObjectFS("s3://other/org/b")
	if err != nil {
		t.Fatal(err)
	}
	ca := a.store.(*cachedStore).objectStore.(*minioStore).client
	cb := b.store.(*cachedStore).objectStore.(*minioStore).client
	if ca != cb {
		t.Fatal("two roots on one store built two clients")
	}
}

// waitEncoded waits for the background encoding of id to land.
func waitEncoded(t *testing.T, c *objectCache, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if e := c.get(id); e != nil && e.tried {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s was never encoded", id)
}

// TestCacheMissFloodKeepsObjects: absences have their own bound, so requests
// for any number of paths that do not exist push out no object.
func TestCacheMissFloodKeepsObjects(t *testing.T) {
	store := newLiveStore(map[string][]byte{"org/site/index.html": []byte("<title>kept</title>")})
	c, _ := testCache(1<<20, 1<<20)
	c.limits[1] = 64 * (entryOverhead + 64)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})

	body(t, srv, "/")
	for i := range 500 {
		if resp, _ := body(t, srv, fmt.Sprintf("/nothing-%d", i)); resp.StatusCode != 404 {
			t.Fatalf("GET /nothing-%d = %d", i, resp.StatusCode)
		}
	}
	if c.bytes[1] > c.limits[1] {
		t.Fatalf("absences hold %d bytes over their limit of %d", c.bytes[1], c.limits[1])
	}
	stats := store.stats.Load()
	if _, got := body(t, srv, "/"); got != "<title>kept</title>" || store.stats.Load() != stats {
		t.Fatal("a miss flood pushed the held page out")
	}
}

// TestCacheReadMatchesHead: a read answered from a copy one write behind fails
// its If-Match, so old bytes are never filed under the new version, and the
// next request after fresh serves the new build.
func TestCacheReadMatchesHead(t *testing.T) {
	a := []byte("<html>build A " + strings.Repeat("a", 2000) + "</html>")
	b := []byte("<html>build B " + strings.Repeat("b", 2000) + "</html>")
	store := newLiveStore(map[string][]byte{"org/site/index.html": a})
	c, clk := testCache(1<<20, 1<<20)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})

	body(t, srv, "/index.html")
	store.publish("org/site/index.html", b)
	store.mu.Lock()
	store.lag["org/site/index.html"] = a
	store.mu.Unlock()
	clk.advance(2 * time.Second)
	body(t, srv, "/index.html")
	clk.advance(2 * time.Second)
	if _, got := body(t, srv, "/index.html"); got != string(b) {
		t.Fatal("after one stale read the old build is still served")
	}
	if e := c.get("sites/org/site/index.html"); e == nil || !bytes.Equal(e.body, b) || e.info.etag != etagOf(b) {
		t.Fatal("the held entry is not build B under B's ETag")
	}
}

// TestCacheFailedReadIsReadAgain: a read that failed once is retried, and
// once it lands the object is held like any other.
func TestCacheFailedReadIsReadAgain(t *testing.T) {
	page := []byte("<html>" + strings.Repeat("p", 4000) + "</html>")
	store := newLiveStore(map[string][]byte{"org/site/index.html": page})
	store.fail["org/site/index.html"] = 1
	c, clk := testCache(1<<20, 1<<20)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})

	body(t, srv, "/index.html")
	clk.advance(2 * time.Second)
	body(t, srv, "/index.html")
	opens := store.opens.Load()
	for range 10 {
		if _, got := body(t, srv, "/index.html"); got != string(page) {
			t.Fatal("wrong body")
		}
	}
	if d := store.opens.Load() - opens; d != 0 {
		t.Fatalf("10 requests after the read recovered cost %d reads, want 0", d)
	}
}

// TestCacheBrotliPreconditionFailed: a failed precondition on the brotli
// representation is a whole 412, not a body promised and never sent.
func TestCacheBrotliPreconditionFailed(t *testing.T) {
	page := []byte("<!doctype html>" + strings.Repeat("<p>hanzo</p>", 400))
	store := newLiveStore(map[string][]byte{"org/site/index.html": page})
	c, _ := testCache(1<<20, 1<<20)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})
	body(t, srv, "/")
	waitEncoded(t, c, "sites/org/site/index.html")

	for _, h := range [][]string{
		{"Accept-Encoding", "br", "If-Match", `"nope"`},
		{"Accept-Encoding", "br", "If-Unmodified-Since", "Mon, 01 Jan 2001 00:00:00 GMT"},
	} {
		resp, got := body(t, srv, "/", h...)
		if resp.StatusCode != http.StatusPreconditionFailed {
			t.Fatalf("%v = %d, want 412", h, resp.StatusCode)
		}
		if resp.Header.Get("Content-Encoding") != "" {
			t.Fatalf("%v: a 412 labelled %q", h, resp.Header.Get("Content-Encoding"))
		}
		if cl := resp.Header.Get("Content-Length"); cl != "" && cl != fmt.Sprint(len(got)) {
			t.Fatalf("%v: Content-Length %s for a %d-byte body", h, cl, len(got))
		}
	}
	if resp, _ := body(t, srv, "/", "Accept-Encoding", "br"); resp.Header.Get("Accept-Ranges") != "" {
		t.Fatalf("the brotli representation offers ranges: %q", resp.Header.Get("Accept-Ranges"))
	}
}

// TestCacheEncodersBounded: encodings beyond the free slots are skipped, not
// queued, and the next request for the object tries again.
func TestCacheEncodersBounded(t *testing.T) {
	c, _ := testCache(1<<20, 1<<20)
	for range cap(c.encoders) {
		c.encoders <- struct{}{}
	}
	e := &cacheEntry{id: "sites/org/site/a.js", info: objectInfo{etag: "x"}, body: bytes.Repeat([]byte("var a=1;"), 512)}
	c.put(e)
	c.encode(e, "org/site/a.js")
	if len(c.pending) != 0 || c.get(e.id).tried {
		t.Fatal("an encoding started with no free slot")
	}
	<-c.encoders
	c.encode(c.get(e.id), "org/site/a.js")
	waitEncoded(t, c, e.id)
	if c.get(e.id).br == nil {
		t.Fatal("the retried encoding did not land")
	}
}

// TestCacheFillsBounded: with every fill slot taken, a cold object streams and
// holds no bytes; once a slot is free, the refresh after fresh reads it.
func TestCacheFillsBounded(t *testing.T) {
	store := newLiveStore(map[string][]byte{"org/site/a.css": []byte("a{color:red}")})
	c, clk := testCache(1<<20, 1<<20)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{})
	for range cap(c.fills) {
		c.fills <- struct{}{}
	}

	if _, got := body(t, srv, "/a.css"); got != "a{color:red}" {
		t.Fatalf("streamed body = %q", got)
	}
	if e := c.get("sites/org/site/a.css"); e == nil || e.body != nil {
		t.Fatal("a fill ran with no free slot")
	}
	for range cap(c.fills) {
		<-c.fills
	}
	clk.advance(2 * time.Second)
	body(t, srv, "/a.css")
	opens := store.opens.Load()
	body(t, srv, "/a.css")
	if store.opens.Load() != opens {
		t.Fatal("the object was not held once a slot was free")
	}
}

// TestCacheEncodingSurvivesRevalidation: an unchanged object revalidated while
// its encoding runs still gets its twin.
func TestCacheEncodingSurvivesRevalidation(t *testing.T) {
	c, _ := testCache(1<<20, 1<<20)
	e := &cacheEntry{id: "sites/org/site/a.js", info: objectInfo{etag: "x", size: 4096}, body: bytes.Repeat([]byte("var a=1;"), 512)}
	c.put(e)
	again := *e
	again.checked = time.Now()
	c.put(&again)
	c.land(e, []byte("br"))
	if got := c.get(e.id); string(got.br) != "br" || !got.tried {
		t.Fatal("the encoding was dropped by a revalidation of the same version")
	}

	moved := *c.get(e.id)
	moved.info.etag, moved.body, moved.br, moved.tried = "y", bytes.Repeat([]byte("b"), 4096), nil, false
	c.put(&moved)
	c.land(e, []byte("old"))
	if got := c.get(e.id); got.br != nil || got.tried {
		t.Fatal("an encoding of the old version landed on the new one")
	}
}
