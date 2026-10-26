package staticfiles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
	"golang.org/x/net/html"
)

const testKey = "pk-0123456789abcdefXYZ_-"

// eventTag is what a page carries for testKey.
const eventTag = `<script defer src="https://api.hanzo.ai/v1/event/tag.js" data-key="pk-0123456789abcdefXYZ_-" data-banner></script>`

const tagPage = "<!doctype html><html><HEAD><title>home</title></HEAD><body>hello</body></html>"

// taggedPage is tagPage as served with testKey.
var taggedPage = strings.Replace(tagPage, "</HEAD>", eventTag+"</HEAD>", 1)

func writeRoot(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, b := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// dom flattens a page as a browser parses it, leaving out the event tag, and
// reports how many event tags it holds and the element holding the first. The
// byte order mark goes first, as a browser's decoder drops it.
func dom(t *testing.T, page string) (flat []string, tags int, parent string) {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(strings.TrimPrefix(page, "\xEF\xBB\xBF")))
	if err != nil {
		t.Fatal(err)
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.ElementNode:
				var attrs []string
				src := ""
				for _, a := range c.Attr {
					attrs = append(attrs, a.Key+"="+a.Val)
					if a.Key == "src" {
						src = a.Val
					}
				}
				if c.Data == "script" && src == eventSrc && strings.Contains(strings.Join(attrs, " "), testKey) {
					if tags++; tags == 1 {
						parent = n.Data
					}
					continue
				}
				flat = append(flat, "<"+c.Data+" "+strings.Join(attrs, " ")+">")
				walk(c)
				flat = append(flat, "</"+c.Data+">")
			case html.TextNode:
				flat = append(flat, "T:"+c.Data)
			case html.CommentNode:
				flat = append(flat, "C:"+c.Data)
			}
		}
	}
	walk(doc)
	return flat, tags, parent
}

// TestTagPlacedInTheHead: wherever markup only looks like the end of the head
// (a script, JSON-LD, a style, a comment, an attribute, a title, a template, a
// noscript), the tag still lands in the page's head as one real element and the
// rest of the page parses exactly as before. A page that leaves out </head>, as
// minifiers do, takes the tag before the first thing that opens its body.
func TestTagPlacedInTheHead(t *testing.T) {
	pages := map[string]string{
		"plain":                 `<!doctype html><html><head><title>t</title></head><body><p>hi</p></body></html>`,
		"upper":                 `<!DOCTYPE html><HTML><HEAD><TITLE>t</TITLE></HEAD><BODY><p>hi</p></BODY></HTML>`,
		"inline-script":         `<!doctype html><html><head><script>var tpl = "<html><head></head></html>"; window.ok = 1;</script><title>t</title></head><body><p>hi</p></body></html>`,
		"jsonld":                `<!doctype html><html><head><script type="application/ld+json">{"headline":"Why your </head> tag matters"}</script></head><body><p>hi</p></body></html>`,
		"style":                 `<!doctype html><html><head><style>code::after{content:"</head>"}</style></head><body><p>hi</p></body></html>`,
		"comment":               `<!doctype html><html><head><!-- </head> moved below --><title>t</title></head><body><p>hi</p></body></html>`,
		"attribute":             `<!doctype html><html><head><meta name="description" content="Paste the snippet before </head> on every page"></head><body><p>hi</p></body></html>`,
		"title":                 `<!doctype html><html><head><title>Why your </head> tag matters</title></head><body><p>hi</p></body></html>`,
		"template":              `<!doctype html><html><head><template id="x"><head></head></template></head><body><p>hi</p></body></html>`,
		"noscript":              `<!doctype html><html><head><noscript><img src="/px?</head>"></noscript></head><body><p>hi</p></body></html>`,
		"cdata":                 "<!doctype html><html><head><script>//<![CDATA[\nvar s='</head>';\n//]]></script></head><body><p>hi</p></body></html>",
		"end-tag-space":         "<!doctype html><html><head><title>t</title></head ><body><p>hi</p></body></html>",
		"end-tag-newline":       "<!doctype html><html><head><title>t</title></head\n><body><p>hi</p></body></html>",
		"two-head-ends":         `<!doctype html><html><head><title>t</title></head><body><p>hi</p></head></body></html>`,
		"bom":                   "\xEF\xBB\xBF<!doctype html><html><head><title>日本語 K</title></head><body><p>é</p></body></html>",
		"omitted-end-body":      `<!doctype html><html><head><title>t</title><body><textarea></head></textarea><p>hi</p></body></html>`,
		"omitted-end-paragraph": `<!doctype html><title>t</title><p>hi</p><script>var tpl = "<head></head>"; window.ok = 1;</script>`,
		"omitted-end-text":      "<!doctype html><title>t</title>\n  hello <b>world</b>",
		"no-head-at-all":        `<p>hi</p>`,
		"whitespace-reference":  `<!doctype html><head>&#32;<title>t</title></head><body>x</body>`,
		"pretty":                "<!doctype html>\n<html>\n  <head>\n    <title>t</title>\n  </head>\n  <body>x</body>\n</html>\n",
		"bom-text":              "\xEF\xBB\xBFhello <b>x</b>",
		"end-body-in-head":      `<!doctype html><title>t</title></body>x`,
		"end-br-in-head":        `<!doctype html><title>t</title></br>`,
	}
	for name, page := range pages {
		srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: writeRoot(t, map[string]string{"p.html": page}), Tag: testKey}))
		resp, got := body(t, srv, "/p.html")
		srv.Close()

		want, _, _ := dom(t, page)
		flat, tags, parent := dom(t, got)
		switch {
		case resp.StatusCode != http.StatusOK:
			t.Errorf("%s: status %d", name, resp.StatusCode)
		case tags != 1 || parent != "head":
			t.Errorf("%s: %d event tags, first in <%s>; want one, in <head>: %q", name, tags, parent, got)
		case strings.Join(flat, "|") != strings.Join(want, "|"):
			t.Errorf("%s: the page parses differently with the tag:\n  was %q\n  now %q", name, want, flat)
		case resp.Header.Get("Content-Length") != fmt.Sprint(len(got)) || len(got) != len(page)+len(eventTag):
			t.Errorf("%s: Content-Length %s for %d bytes", name, resp.Header.Get("Content-Length"), len(got))
		}
	}
}

// TestTagServedAsIs: a page that already loads the tag, one whose head never
// ends, and anything that is not an HTML page are sent byte for byte, with the
// file's own validators.
func TestTagServedAsIs(t *testing.T) {
	files := map[string]string{
		"tagged.html":    `<html><head><script src="https://api.hanzo.ai/v1/event/tag.js" data-key="pk-own"></script></head></html>`,
		"in-body.html":   `<html><head></head><body><script defer src="https://api.hanzo.ai/v1/event/tag.js" data-key="pk-own"></script></body></html>`,
		"open-head.html": `<!doctype html><html><head><title>t</title>`,
		"app.js":         "document.write('</head>')",
		"data.json":      `{"html":"</head>"}`,
		"notes.txt":      "</head>",
		"logo.svg":       "<svg><head></head></svg>",
		"LICENSE":        "Plain text that mentions </head>.",
		"utf16le.html":   "\xFF\xFE<\x00h\x00e\x00a\x00d\x00>\x00<\x00/\x00h\x00e\x00a\x00d\x00>\x00",
		"utf16be.html":   "\xFE\xFF\x00<\x00h\x00e\x00a\x00d\x00>\x00<\x00/\x00h\x00e\x00a\x00d\x00>",
		"foreign.html":   `<html><head><template><svg><![CDATA[ a > b </template> ]]></svg></template></head><body></body></html>`,
	}
	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: writeRoot(t, files), Tag: testKey}))
	defer srv.Close()

	for name, want := range files {
		resp, got := body(t, srv, "/"+name)
		if resp.StatusCode != http.StatusOK || got != want {
			t.Errorf("GET /%s = %d %q, want it as it is", name, resp.StatusCode, got)
		}
		if resp.Header.Get("Last-Modified") == "" {
			t.Errorf("GET /%s lost its Last-Modified", name)
		}
	}
	// The 404 page is read to place the tag, and then sent whole.
	miss := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: writeRoot(t, files), ErrorPage404: "tagged.html", Tag: testKey}))
	defer miss.Close()
	if resp, got := body(t, miss, "/nope"); resp.StatusCode != http.StatusNotFound || got != files["tagged.html"] {
		t.Errorf("404 page = %d %q, want it as it is", resp.StatusCode, got)
	}
}

// TestTagMentionIsNotATag: only a <script> loading the tag counts as one. A page
// that shows the tag's address in text, or holds it in an inert template, takes
// the tag.
func TestTagMentionIsNotATag(t *testing.T) {
	files := map[string]string{
		"docs.html":     `<!doctype html><html><head><title>Install</title></head><body><pre><code>&lt;script defer src="https://api.hanzo.ai/v1/event/tag.js" data-key="pk-..."&gt;&lt;/script&gt;</code></pre></body></html>`,
		"template.html": `<!doctype html><html><head><title>t</title></head><body><template><script src="https://api.hanzo.ai/v1/event/tag.js"></script></template></body></html>`,
	}
	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: writeRoot(t, files), Tag: testKey}))
	defer srv.Close()

	for name, page := range files {
		if _, got := body(t, srv, "/"+name); got != strings.Replace(page, "</head>", eventTag+"</head>", 1) {
			t.Errorf("GET /%s = %q, want it tagged", name, got)
		}
	}
}

// TestTagExtensionlessPage: a file with no extension is a page by its name, and
// HTML or not by what ServeContent sniffs; only the HTML one takes the tag.
func TestTagExtensionlessPage(t *testing.T) {
	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: writeRoot(t, map[string]string{"about": tagPage}), Tag: testKey}))
	defer srv.Close()

	resp, got := body(t, srv, "/about")
	if got != taggedPage || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("GET /about = %q (%s), want the tagged page as HTML", got, resp.Header.Get("Content-Type"))
	}
}

// TestTagKey: the tag is rendered from a publishable key only. Anything else,
// a secret key included, builds the middleware and serves pages without it.
func TestTagKey(t *testing.T) {
	if tag, _ := eventOf(testKey); string(tag) != eventTag {
		t.Fatalf("eventOf = %s, want %s", tag, eventTag)
	}
	for _, key := range []string{"sk-0123456789abcdefXYZ", "pk-short", `pk-0123456789abcdef"><script>`, "pk-0123456789abcdef ", " pk-0123456789abcdef", "PK-0123456789abcdefgh", "pk-" + strings.Repeat("a", 129)} {
		h, err := New(context.Background(), http.NotFoundHandler(), dynamic.StaticFiles{Root: writeRoot(t, map[string]string{"index.html": tagPage}), Tag: key}, "tag")
		if err != nil {
			t.Fatalf("key %q refused the build: %v", key, err)
		}
		srv := httptest.NewServer(h)
		if _, got := body(t, srv, "/"); got != tagPage {
			t.Errorf("key %q: served %q, want the page untagged", key, got)
		}
		srv.Close()
	}
}

// TestTagObjectValidators: on the object store the page's ETag is the file's
// with the tag's sum, so a revalidation and a range answer from the page sent,
// and one against the file's ETag gets the page whole. A date proves nothing
// about the tag, so none is sent and none is compared.
func TestTagObjectValidators(t *testing.T) {
	store := newMapStore(map[string][]byte{"cd/index.html": []byte(tagPage)})
	fileTag := `"` + etagOf([]byte(tagPage)) + `"`
	sum := sha256.Sum256([]byte("v1\x00" + eventTag))
	pageTag := strings.TrimSuffix(fileTag, `"`) + "-" + hex.EncodeToString(sum[:8]) + `"`
	srv := httptest.NewServer(s3Handler(store, "cd", dynamic.StaticFiles{Tag: testKey}))
	defer srv.Close()

	resp, got := body(t, srv, "/")
	if got != taggedPage || resp.Header.Get("ETag") != pageTag || resp.Header.Get("Last-Modified") != "" {
		t.Fatalf("GET / = %q ETag %s Last-Modified %q; want the page under %s and no date", got, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), pageTag)
	}
	if cl := resp.Header.Get("Content-Length"); cl != fmt.Sprint(len(taggedPage)) {
		t.Fatalf("Content-Length = %s, want %d", cl, len(taggedPage))
	}

	if resp, _ := body(t, srv, "/", "If-None-Match", pageTag); resp.StatusCode != http.StatusNotModified {
		t.Fatalf("If-None-Match page ETag = %d, want 304", resp.StatusCode)
	}
	if resp, got := body(t, srv, "/", "If-None-Match", fileTag); resp.StatusCode != http.StatusOK || got != taggedPage {
		t.Fatalf("If-None-Match file ETag = %d %q, want 200 page", resp.StatusCode, got)
	}
	future := "Mon, 01 Jan 2125 00:00:00 GMT"
	if resp, got := body(t, srv, "/", "If-Modified-Since", future); resp.StatusCode != http.StatusOK || got != taggedPage {
		t.Fatalf("If-Modified-Since = %d, want 200 page", resp.StatusCode)
	}

	// A range across the tag is the page's bytes, never the file's.
	at := strings.Index(taggedPage, "<script")
	rng := fmt.Sprintf("bytes=%d-%d", at-4, at+20)
	want := taggedPage[at-4 : at+21]
	resp, got = body(t, srv, "/", "Range", rng)
	if resp.StatusCode != http.StatusPartialContent || got != want {
		t.Fatalf("Range %s = %d %q, want 206 %q", rng, resp.StatusCode, got, want)
	}
	if cr := resp.Header.Get("Content-Range"); cr != fmt.Sprintf("bytes %d-%d/%d", at-4, at+20, len(taggedPage)) {
		t.Fatalf("Content-Range = %q, want the page's length", cr)
	}
	if resp, got := body(t, srv, "/", "Range", rng, "If-Range", pageTag); resp.StatusCode != http.StatusPartialContent || got != want {
		t.Fatalf("If-Range page ETag = %d %q, want 206", resp.StatusCode, got)
	}
	for _, ir := range []string{fileTag, future} {
		if resp, got := body(t, srv, "/", "Range", rng, "If-Range", ir); resp.StatusCode != http.StatusOK || got != taggedPage {
			t.Fatalf("If-Range %s = %d %q, want 200 page", ir, resp.StatusCode, got)
		}
	}

	req, _ := http.NewRequest(http.MethodHead, srv.URL+"/", nil)
	head, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := io.Copy(io.Discard, head.Body)
	head.Body.Close()
	if head.StatusCode != http.StatusOK || n != 0 || head.ContentLength != int64(len(taggedPage)) {
		t.Fatalf("HEAD / = %d, %d body bytes, Content-Length %d; want 200, 0, %d", head.StatusCode, n, head.ContentLength, len(taggedPage))
	}

	// Another key is another page.
	other := httptest.NewServer(s3Handler(store, "cd", dynamic.StaticFiles{Tag: "pk-another0123456789"}))
	defer other.Close()
	if resp, _ := body(t, other, "/"); resp.Header.Get("ETag") == pageTag || resp.Header.Get("ETag") == fileTag {
		t.Fatal("two keys served one ETag")
	}
}

// TestTagReadOncePerVersion: the cache remembers where a page takes the tag, so
// revalidations, HEADs and ranges of a large page cost what an untagged one
// does, and a publish is placed again.
func TestTagReadOncePerVersion(t *testing.T) {
	page := []byte("<!doctype html><html><head><title>big</title></head><body>" + strings.Repeat("<p>hanzo</p>", 120000) + "</body></html>")
	store := newLiveStore(map[string][]byte{"org/site/index.html": page})
	c, clk := testCache(64<<20, headMax)
	h := s3Handler(&cachedStore{objectStore: store, bucket: "sites", cache: c}, "org/site", dynamic.StaticFiles{Tag: testKey})

	serve := func(header ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		for i := 0; i+1 < len(header); i += 2 {
			r.Header.Set(header[i], header[i+1])
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	tag := serve().Header().Get("ETag")
	// The encoding runs behind the first request; it is not what is measured.
	waitEncoded(t, c, "sites/org/site/index.html")
	if e := c.get("sites/org/site/index.html"); e == nil || !e.placed || e.at != int64(bytes.Index(page, []byte("</head>"))) {
		t.Fatalf("placement not remembered: %+v", e)
	}

	for name, header := range map[string][]string{
		"304":   {"If-None-Match", tag},
		"range": {"Range", "bytes=0-0"},
	} {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for range 20 {
			serve(header...)
		}
		runtime.ReadMemStats(&after)
		if per := (after.TotalAlloc - before.TotalAlloc) / 20; per > 64<<10 {
			t.Errorf("%s: %d bytes allocated per request on a %d-byte page", name, per, len(page))
		}
	}

	store.publish("org/site/index.html", []byte(tagPage))
	clk.advance(2 * time.Second)
	if got := serve().Body.String(); got != taggedPage {
		t.Fatalf("after a publish: %q, want the new page tagged", got)
	}
}

// TestTagSkipsTheEncodedTwin: the brotli twin holds the file's bytes, so a page
// with the tag goes as identity even to a reader that takes br.
func TestTagSkipsTheEncodedTwin(t *testing.T) {
	page := "<!doctype html><html><head><title>br</title></head><body>" + strings.Repeat("<p>hanzo</p>", 400) + "</body></html>"
	store := newLiveStore(map[string][]byte{"org/site/index.html": []byte(page)})
	c, _ := testCache(1<<20, 1<<20)
	srv := cachedServer(t, store, c, dynamic.StaticFiles{Tag: testKey})
	body(t, srv, "/")
	waitEncoded(t, c, "sites/org/site/index.html")

	resp, got := body(t, srv, "/", "Accept-Encoding", "br")
	if enc := resp.Header.Get("Content-Encoding"); enc != "" || got != strings.Replace(page, "</head>", eventTag+"</head>", 1) {
		t.Fatalf("br reader got Content-Encoding %q, %d bytes; want the identity page", enc, len(got))
	}
	if cl := resp.Header.Get("Content-Length"); cl != fmt.Sprint(len(got)) {
		t.Fatalf("Content-Length = %s for %d bytes", cl, len(got))
	}
	if strings.HasSuffix(resp.Header.Get("ETag"), `-br"`) {
		t.Fatalf("ETag %s names the encoded file", resp.Header.Get("ETag"))
	}
}

// TestTagSPAAndNotFoundPages: the shell an SPA answers a route with, and the
// site's 404 page, take the tag; the 404 keeps its status and its length is the
// page's.
func TestTagSPAAndNotFoundPages(t *testing.T) {
	root := writeRoot(t, map[string]string{"index.html": tagPage, "404.html": tagPage})

	spa := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: root, SPAMode: true, Tag: testKey}))
	defer spa.Close()
	if resp, got := body(t, spa, "/deep/client/route"); resp.StatusCode != http.StatusOK || got != taggedPage {
		t.Fatalf("SPA route = %d %q, want 200 tagged shell", resp.StatusCode, got)
	}

	miss := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: root, ErrorPage404: "404.html", Tag: testKey}))
	defer miss.Close()
	resp, got := body(t, miss, "/nope")
	if resp.StatusCode != http.StatusNotFound || got != taggedPage || resp.Header.Get("Content-Length") != fmt.Sprint(len(taggedPage)) {
		t.Fatalf("404 page = %d %q (Content-Length %s), want 404 tagged page", resp.StatusCode, got, resp.Header.Get("Content-Length"))
	}
	head, err := miss.Client().Head(miss.URL + "/nope")
	if err != nil {
		t.Fatal(err)
	}
	head.Body.Close()
	if head.StatusCode != http.StatusNotFound || head.ContentLength != int64(len(taggedPage)) {
		t.Fatalf("HEAD 404 page = %d Content-Length %d, want 404 %d", head.StatusCode, head.ContentLength, len(taggedPage))
	}
}

// TestTagSizeCap: a page over headMax is not read for the tag; it is sent as it
// is.
func TestTagSizeCap(t *testing.T) {
	big := "<html><head></head><body>" + strings.Repeat("x", headMax) + "</body></html>"
	srv := httptest.NewServer(newLocal(t, dynamic.StaticFiles{Root: writeRoot(t, map[string]string{"index.html": big}), Tag: testKey}))
	defer srv.Close()

	resp, got := body(t, srv, "/")
	if got != big || resp.Header.Get("Content-Length") != fmt.Sprint(len(big)) {
		t.Fatalf("page over the cap: %d bytes (Content-Length %s), want %d as it is", len(got), resp.Header.Get("Content-Length"), len(big))
	}
}

// TestTagAbsentIsUnchanged: with no key the response is the file's, headers and
// all.
func TestTagAbsentIsUnchanged(t *testing.T) {
	store := newMapStore(map[string][]byte{"cd/index.html": []byte(tagPage)})
	srv := httptest.NewServer(s3Handler(store, "cd", dynamic.StaticFiles{}))
	defer srv.Close()

	resp, got := body(t, srv, "/")
	if got != tagPage || resp.Header.Get("ETag") != `"`+etagOf([]byte(tagPage))+`"` || resp.Header.Get("Last-Modified") == "" {
		t.Fatalf("no key: %q ETag %s Last-Modified %q, want the file", got, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"))
	}
}

// TestTagReadsWhatItPlaces: a page that reads past headMax, whatever its stat
// said, is served as it is, since the tag is placed only in a page read whole.
func TestTagReadsWhatItPlaces(t *testing.T) {
	page := []byte("<html><head></head><body>" + strings.Repeat("x", headMax) + "</body></html>")
	srv := httptest.NewServer(s3Handler(&shortStat{newMapStore(map[string][]byte{"cd/index.html": page})}, "cd", dynamic.StaticFiles{Tag: testKey}))
	defer srv.Close()

	if _, got := body(t, srv, "/"); got != string(page) {
		t.Fatalf("served %d bytes, want the %d-byte page as it is", len(got), len(page))
	}
}

// shortStat reports every object at one byte, as a stat that raced a publish can.
type shortStat struct{ *mapStore }

func (s *shortStat) stat(ctx context.Context, key string) (objectInfo, error) {
	info, err := s.mapStore.stat(ctx, key)
	info.size = 1
	return info, err
}

// cutStore serves objects whose reads fail partway and stay failed, as a reset
// GET does.
type cutStore struct{ page []byte }

func (s *cutStore) stat(_ context.Context, key string) (objectInfo, error) {
	if key != "cd/index.html" && key != "cd/404.html" {
		return objectInfo{}, fs.ErrNotExist
	}
	return objectInfo{key: key, size: int64(len(s.page)), etag: etagOf(s.page)}, nil
}

func (s *cutStore) open(context.Context, string, string) (readSeekCloser, error) {
	return &cutReader{r: bytes.NewReader(s.page), left: len(s.page) / 2}, nil
}

func (s *cutStore) list(context.Context, string) ([]objectInfo, error) { return nil, nil }

type cutReader struct {
	r    *bytes.Reader
	left int
	err  error
}

func (c *cutReader) Read(p []byte) (int, error) {
	if c.err == nil && c.left == 0 {
		c.err = errors.New("connection reset by peer")
	}
	if c.err != nil {
		return 0, c.err
	}
	n, err := c.r.Read(p[:min(len(p), c.left)])
	c.left -= n
	return n, err
}

func (c *cutReader) Seek(off int64, whence int) (int64, error) {
	if c.err != nil {
		return 0, c.err
	}
	return c.r.Seek(off, whence)
}

func (*cutReader) Close() error { return nil }

// TestTagFailedRead: a page whose read fails while it is placed is answered 500,
// on both the page and the 404 path, never with a length its body cannot meet.
func TestTagFailedRead(t *testing.T) {
	page := []byte("<!doctype html><html><head><title>t</title></head><body>" + strings.Repeat("x", 100000) + "</body></html>")
	srv := httptest.NewServer(s3Handler(&cutStore{page: page}, "cd", dynamic.StaticFiles{Tag: testKey, ErrorPage404: "404.html"}))
	defer srv.Close()

	for _, p := range []string{"/index.html", "/missing"} {
		resp, got := body(t, srv, p)
		if resp.StatusCode != http.StatusInternalServerError || resp.Header.Get("Content-Length") != "" && resp.Header.Get("Content-Length") != fmt.Sprint(len(got)) {
			t.Errorf("GET %s = %d, Content-Length %s for %d bytes; want a 500", p, resp.StatusCode, resp.Header.Get("Content-Length"), len(got))
		}
		if cc := resp.Header.Get("Cache-Control"); resp.Header.Get("ETag") != "" || cc != "" && cc != "no-store" {
			t.Errorf("GET %s: a 500 carries the page's ETag %q / Cache-Control %q", p, resp.Header.Get("ETag"), resp.Header.Get("Cache-Control"))
		}
	}
}

// TestSpliceReadsThePage: every seek and read of a splice is the page's bytes,
// across the tag's edges and the ends.
func TestSpliceReadsThePage(t *testing.T) {
	file := []byte("0123456789abcdefghij")
	tag := []byte("<TAG>")
	for at := range int64(len(file)) {
		want := string(file[:at]) + string(tag) + string(file[at:])
		s := &splice{f: bytes.NewReader(file), tag: tag, at: at, size: int64(len(file)), fpos: -1}
		if got, err := io.ReadAll(s); err != nil || string(got) != want {
			t.Fatalf("at %d: ReadAll = %q, %v; want %q", at, got, err, want)
		}
		for range 50 {
			off := rand.IntN(len(want) + 1)
			n := rand.IntN(len(want) + 2)
			if _, err := s.Seek(int64(off), io.SeekStart); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, n)
			got, err := io.ReadFull(s, buf)
			end := min(off+n, len(want))
			if string(buf[:got]) != want[off:end] || (got < n) != (err != nil) {
				t.Fatalf("at %d: read %d at %d = %q, %v; want %q", at, n, off, buf[:got], err, want[off:end])
			}
		}
		if end, _ := s.Seek(0, io.SeekEnd); end != int64(len(want)) {
			t.Fatalf("at %d: end %d, want %d", at, end, len(want))
		}
	}

	// A file shorter than its size is a page cut short, not a shorter page.
	s := &splice{f: bytes.NewReader(file[:5]), tag: tag, at: 8, size: int64(len(file)), fpos: -1}
	if _, err := io.ReadAll(s); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short file: %v, want io.ErrUnexpectedEOF", err)
	}
}
