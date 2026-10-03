package staticfiles

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/bits"
	"mime"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"golang.org/x/sync/singleflight"
)

const (
	// cacheBytes bounds the objects the process holds, across every site.
	cacheBytes = 256 << 20
	// missBytes bounds the absences it holds, apart from the objects, so a flood
	// of requests for paths that do not exist cannot push a single object out.
	missBytes = 4 << 20
	// cacheObjectMax is the largest object held. A larger one streams from the
	// store on every request, as it did before the cache.
	cacheObjectMax = 2 << 20
	// cacheFresh is how long an entry is served without asking the store. Sites
	// publish in place (cloud apps/projects writes <org>/<slug>/ and keeps the
	// keys), so it is also the longest a publish waits to be served.
	cacheFresh = time.Second
	// entryOverhead is charged per entry on top of its bytes.
	entryOverhead = 256
	// brotliQuality trades ratio for time; the encoding runs once per version.
	brotliQuality = 5
)

// objects is the cache every object-store root in the process shares: one bound
// for the process, and a configuration reload, which rebuilds every middleware,
// keeps what it holds.
var objects = newObjectCache(cacheBytes, missBytes, cacheObjectMax, cacheFresh)

// objectCache holds objects as last read from the store, least recently used
// first out. An entry checked within fresh is served from memory; an older one
// costs one HEAD, and its bytes are read again only when the ETag moved. Absence
// is held the same way, in its own bound, so the page ladder's probes ("pricing"
// before "pricing.html") stop costing a round trip each.
type objectCache struct {
	max   int64
	fresh time.Duration
	now   func() time.Time

	mu      sync.Mutex
	index   map[string]*list.Element
	lists   [2]*list.List // held objects, held absences; most recent first
	bytes   [2]int64
	limits  [2]int64
	pending map[string]bool // ids being encoded

	flight singleflight.Group
	// encoders bounds the brotli encodings running at once. An encoding that
	// finds no free slot is not queued; the next request for the object tries
	// again.
	encoders chan struct{}
}

// cacheEntry is one object as last read, or the fact that it was absent. It is
// never changed once stored; a refresh stores a new one.
type cacheEntry struct {
	id      string
	info    objectInfo
	missing bool
	body    []byte // nil when the object is larger than max, or its read failed
	br      []byte // body brotli-encoded, once that has run and paid off
	tried   bool   // the encoding ran, whether or not it paid off
	checked time.Time
}

func (e *cacheEntry) cost() int64 { return int64(len(e.id)+len(e.body)+len(e.br)) + entryOverhead }

func (e *cacheEntry) class() int {
	if e.missing {
		return 1
	}
	return 0
}

func newObjectCache(limit, missLimit, objectMax int64, fresh time.Duration) *objectCache {
	return &objectCache{
		max: objectMax, fresh: fresh, now: time.Now,
		index:    map[string]*list.Element{},
		lists:    [2]*list.List{list.New(), list.New()},
		limits:   [2]int64{limit, missLimit},
		pending:  map[string]bool{},
		encoders: make(chan struct{}, max(1, runtime.GOMAXPROCS(0)/2)),
	}
}

func (c *objectCache) get(id string) *cacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.index[id]
	if !ok {
		return nil
	}
	e := el.Value.(*cacheEntry)
	c.lists[e.class()].MoveToFront(el)
	return e
}

func (c *objectCache) put(e *cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.putLocked(e)
}

// swap stores e only while old is still the entry for its id, so a late writer
// never replaces a newer answer.
func (c *objectCache) swap(old, e *cacheEntry) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.index[e.id]; !ok || el.Value.(*cacheEntry) != old {
		return false
	}
	c.putLocked(e)
	return true
}

func (c *objectCache) putLocked(e *cacheEntry) {
	if el, ok := c.index[e.id]; ok {
		old := el.Value.(*cacheEntry)
		c.lists[old.class()].Remove(el)
		c.bytes[old.class()] -= old.cost()
	}
	k := e.class()
	c.index[e.id] = c.lists[k].PushFront(e)
	c.bytes[k] += e.cost()
	for c.bytes[k] > c.limits[k] {
		el := c.lists[k].Back()
		old := el.Value.(*cacheEntry)
		c.lists[k].Remove(el)
		delete(c.index, old.id)
		c.bytes[k] -= old.cost()
	}
}

// encode brotli-encodes e's body in the background when a slot is free, and
// stores the result while e is still current. A request never waits on it: the
// object is sent as it is until its twin exists.
func (c *objectCache) encode(e *cacheEntry, key string) {
	if e.tried || e.body == nil {
		return
	}
	if len(e.body) < 1024 || !compressible(key) {
		done := *e
		done.tried = true
		c.swap(e, &done)
		return
	}
	c.mu.Lock()
	if c.pending[e.id] {
		c.mu.Unlock()
		return
	}
	select {
	case c.encoders <- struct{}{}:
	default:
		c.mu.Unlock()
		return
	}
	c.pending[e.id] = true
	c.mu.Unlock()

	go func() {
		defer func() { <-c.encoders }()
		done := *e
		done.br, done.tried = brotliOf(e.body), true
		c.mu.Lock()
		delete(c.pending, e.id)
		c.mu.Unlock()
		c.swap(e, &done)
	}()
}

// cachedStore is an objectStore answered from an objectCache where it can. s3FS
// and the handler do not change for it.
type cachedStore struct {
	objectStore
	bucket string
	cache  *objectCache
}

func (s *cachedStore) stat(ctx context.Context, key string) (objectInfo, error) {
	e, err := s.entry(ctx, key)
	if err != nil {
		return objectInfo{}, err
	}
	return e.info, nil
}

func (s *cachedStore) open(ctx context.Context, key, etag string) (readSeekCloser, error) {
	e, err := s.entry(ctx, key)
	if err != nil {
		return nil, err
	}
	if e.body == nil {
		return s.objectStore.open(ctx, key, etag)
	}
	s.cache.encode(e, key)
	return &cachedBody{Reader: bytes.NewReader(e.body), br: e.br}, nil
}

// entry is key's current entry: from memory when it was checked within fresh,
// otherwise from the store, asked once per key however many requests wait.
func (s *cachedStore) entry(ctx context.Context, key string) (*cacheEntry, error) {
	id := s.bucket + "/" + key
	e := s.cache.get(id)
	if e == nil || s.cache.now().Sub(e.checked) >= s.cache.fresh {
		old := e
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: %w", errObjectStoreUnavailable, ctx.Err())
		case r := <-s.cache.flight.DoChan(id, func() (any, error) { return s.refresh(id, key, old) }):
			switch {
			case r.Err == nil:
				e = r.Val.(*cacheEntry)
			case old == nil || !errors.Is(r.Err, errObjectStoreUnavailable):
				return nil, r.Err
			default:
				// The store could not answer and the last answer is held: the
				// site keeps serving through the outage, and the store is asked
				// again once per fresh rather than once per request.
				held := *old
				held.checked = s.cache.now()
				s.cache.swap(old, &held)
				e = &held
			}
		}
	}
	if e.missing {
		return nil, fs.ErrNotExist
	}
	return e, nil
}

// refresh asks the store about key. Absence is held; a store failure is not, so
// an outage is asked about again on the next request. The context is the
// refresh's own, because every request waiting on it shares the answer.
func (s *cachedStore) refresh(id, key string, old *cacheEntry) (*cacheEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), metadataTimeout)
	defer cancel()

	// The newest entry, not the one the first waiter saw: an encoding may have
	// landed since.
	if cur := s.cache.get(id); cur != nil {
		old = cur
	}
	now := s.cache.now()
	info, err := s.objectStore.stat(ctx, key)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		e := &cacheEntry{id: id, missing: true, checked: now}
		s.cache.put(e)
		return e, nil
	case err != nil:
		return nil, err
	}

	held := old != nil && !old.missing && (old.body != nil || info.size > s.cache.max)
	if held && old.info.etag == info.etag && old.info.size == info.size && old.info.modTime.Equal(info.modTime) {
		e := *old
		e.info, e.checked = info, now
		s.cache.put(&e)
		return &e, nil
	}

	e := &cacheEntry{id: id, info: info, checked: now}
	if info.size <= s.cache.max {
		if e.body = s.read(ctx, key, info); e.body == nil {
			// Changed, gone or unreadable between the HEAD and the read. Hold
			// nothing fresh: this request streams from the store, the next asks
			// again.
			e.checked = time.Time{}
		}
	}
	s.cache.put(e)
	return e, nil
}

// read returns the bytes of exactly the version the HEAD saw: the read carries
// its ETag as If-Match, so a store answering from an older copy fails the read
// instead of filing old bytes under the new version. nil when that, or the size,
// does not hold.
func (s *cachedStore) read(ctx context.Context, key string, info objectInfo) []byte {
	rc, err := s.objectStore.open(ctx, key, info.etag)
	if err != nil {
		return nil
	}
	defer rc.Close()
	body, err := io.ReadAll(io.LimitReader(rc, info.size+1))
	if err != nil || int64(len(body)) != info.size {
		return nil
	}
	return body
}

// brotliOf returns body brotli-encoded when that saves a tenth or more, else
// nil. The window is sized to the body, which is what bounds the encoder's
// memory to a small multiple of the input.
func brotliOf(body []byte) []byte {
	var buf bytes.Buffer
	w := brotli.NewWriterOptions(&buf, brotli.WriterOptions{
		Quality: brotliQuality,
		LGWin:   min(max(bits.Len(uint(len(body)-1)), 10), 24),
	})
	if _, err := w.Write(body); err != nil {
		return nil
	}
	if err := w.Close(); err != nil || buf.Len() > len(body)*9/10 {
		return nil
	}
	return buf.Bytes()
}

func compressible(key string) bool {
	t, _, _ := strings.Cut(mime.TypeByExtension(path.Ext(key)), ";")
	switch t = strings.TrimSpace(t); {
	case strings.HasPrefix(t, "text/"), strings.HasSuffix(t, "+json"), strings.HasSuffix(t, "+xml"):
		return true
	}
	switch t {
	case "application/javascript", "application/json", "application/xml", "application/wasm":
		return true
	}
	return false
}

// cachedBody is a held object's bytes as a stream, with its encoded twin.
type cachedBody struct {
	*bytes.Reader
	br []byte
}

func (*cachedBody) Close() error     { return nil }
func (b *cachedBody) brotli() []byte { return b.br }
