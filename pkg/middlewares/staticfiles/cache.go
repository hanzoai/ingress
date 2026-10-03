package staticfiles

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"golang.org/x/sync/singleflight"
)

const (
	// cacheBytes bounds what the process holds, across every site.
	cacheBytes = 256 << 20
	// cacheObjectMax is the largest object held. A larger one streams from the
	// store on every request, as it did before the cache.
	cacheObjectMax = 2 << 20
	// cacheFresh is how long an entry is served without asking the store. Sites
	// publish in place (cloud apps/projects writes <org>/<slug>/ and keeps the
	// keys), so it is also the longest a publish waits to be served.
	cacheFresh = time.Second
	// entryOverhead is charged per entry on top of its bytes, so a stream of
	// distinct misses is bounded like everything else.
	entryOverhead = 256
)

// objects is the cache every object-store root in the process shares: one bound
// for the process, and a configuration reload, which rebuilds every middleware,
// keeps what it holds.
var objects = newObjectCache(cacheBytes, cacheObjectMax, cacheFresh)

// objectCache holds objects as last read from the store, least recently used
// first out. An entry checked within fresh is served from memory; an older one
// costs one HEAD, and its bytes are read again only when the ETag moved. Absence
// is held the same way, so the page ladder's probes ("pricing" before
// "pricing.html") stop costing a round trip each.
type objectCache struct {
	limit, max int64
	fresh      time.Duration
	now        func() time.Time

	mu    sync.Mutex
	lru   *list.List // of *cacheEntry, most recent first
	index map[string]*list.Element
	bytes int64

	flight singleflight.Group
}

// cacheEntry is one object as last read, or the fact that it was absent. It is
// never changed once stored; a refresh stores a new one.
type cacheEntry struct {
	id      string
	info    objectInfo
	missing bool
	body    []byte // nil when the object is larger than max, or its read failed
	br      []byte // body brotli-encoded, when its type compresses
	checked time.Time
}

func (e *cacheEntry) cost() int64 { return int64(len(e.id)+len(e.body)+len(e.br)) + entryOverhead }

func newObjectCache(limit, max int64, fresh time.Duration) *objectCache {
	return &objectCache{limit: limit, max: max, fresh: fresh, now: time.Now, lru: list.New(), index: map[string]*list.Element{}}
}

func (c *objectCache) get(id string) *cacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.index[id]
	if !ok {
		return nil
	}
	c.lru.MoveToFront(el)
	return el.Value.(*cacheEntry)
}

func (c *objectCache) put(e *cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.index[e.id]; ok {
		c.bytes -= el.Value.(*cacheEntry).cost()
		el.Value = e
		c.lru.MoveToFront(el)
	} else {
		c.index[e.id] = c.lru.PushFront(e)
	}
	c.bytes += e.cost()
	for c.bytes > c.limit {
		el := c.lru.Back()
		old := el.Value.(*cacheEntry)
		c.lru.Remove(el)
		delete(c.index, old.id)
		c.bytes -= old.cost()
	}
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

func (s *cachedStore) open(ctx context.Context, key string) (readSeekCloser, error) {
	e, err := s.entry(ctx, key)
	if err != nil {
		return nil, err
	}
	if e.body == nil {
		return s.objectStore.open(ctx, key)
	}
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
				s.cache.put(&held)
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

	if old != nil && !old.missing && old.info.etag == info.etag && old.info.size == info.size && old.info.modTime.Equal(info.modTime) {
		e := *old
		e.info, e.checked = info, now
		s.cache.put(&e)
		return &e, nil
	}

	e := &cacheEntry{id: id, info: info, checked: now}
	if info.size <= s.cache.max {
		if e.body = s.read(ctx, key, info.size); e.body == nil {
			// Overwritten or gone between the HEAD and the read. Hold nothing
			// fresh: this request streams from the store, the next asks again.
			e.checked = time.Time{}
		}
		e.br = encode(key, e.body)
	}
	s.cache.put(e)
	return e, nil
}

// read returns the object's bytes, or nil when they are not the size the HEAD
// reported or cannot be read.
func (s *cachedStore) read(ctx context.Context, key string, size int64) []byte {
	rc, err := s.objectStore.open(ctx, key)
	if err != nil {
		return nil
	}
	defer rc.Close()
	body, err := io.ReadAll(io.LimitReader(rc, size+1))
	if err != nil || int64(len(body)) != size {
		return nil
	}
	return body
}

// encode returns body brotli-encoded when its type is text and the encoding
// saves a tenth or more, else nil. It runs once per object version, not per
// request.
func encode(key string, body []byte) []byte {
	if len(body) < 1024 || !compressible(key) {
		return nil
	}
	var buf bytes.Buffer
	w := brotli.NewWriterLevel(&buf, brotli.DefaultCompression)
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
