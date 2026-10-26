package staticfiles

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const (
	// headMax is the largest page that takes the event tag. It is the object
	// cache's bound, so a page that takes it is one the cache can hold.
	headMax = cacheObjectMax
	// eventSrc is Hanzo's hosted event tag.
	eventSrc = "https://api.hanzo.ai/v1/event/tag.js"
	// eventRule names the rules that render and place the tag. A tagged page's
	// ETag carries it, so changing either rule must change it too.
	eventRule = "v1"
)

// publishable is the shape of a site's publishable key.
// A minted key is 46 bytes; the bound keeps a mistyped value from bloating every page.
var publishable = regexp.MustCompile(`^pk-[A-Za-z0-9_-]{16,128}$`)

// eventOf renders the event tag for a publishable key, with the sum a tagged
// page's ETag adds. It is nil for an empty key or one that is not publishable,
// so a secret pasted here is never written into a public page.
func eventOf(key string) ([]byte, string) {
	if !publishable.MatchString(key) {
		return nil, ""
	}
	tag := []byte(`<script defer src="` + eventSrc + `" data-key="` + html.EscapeString(key) + `" data-banner></script>`)
	sum := sha256.Sum256(append([]byte(eventRule+"\x00"), tag...))
	return tag, hex.EncodeToString(sum[:8])
}

// placed is a file that remembers where its page takes the tag, as the object
// cache's do: an offset, or -1 for a page served as it is.
type placed interface {
	placement() (int64, bool)
	place(at int64)
}

// tagged returns the page f holds with the event tag in it, or nil when the page
// is served as it is. Where the tag goes is read from the page once per version
// when the origin remembers it (the object cache) and on every request when it
// does not, so a revalidation, a HEAD or a range of a remembered page reads
// none of it. An error is a read of f that failed; nothing has been written.
func (h *staticFiles) tagged(f http.File, d fs.FileInfo) (*splice, error) {
	if h.event == nil || kindOf(d.Name()) != page || d.Size() > headMax {
		return nil, nil
	}
	p, remembers := f.(placed)
	at, ok := int64(0), false
	if remembers {
		at, ok = p.placement()
	}
	if !ok {
		var err error
		if at, err = locate(f, d.Name()); err != nil {
			return nil, err
		}
		if remembers {
			p.place(at)
		}
	}
	if at < 0 {
		// The page goes out as the file, read from its start.
		_, err := f.Seek(0, io.SeekStart)
		return nil, err
	}
	// The size as f reads it: what the page is spliced from.
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	return &splice{f: f, tag: h.event, at: at, size: size, fpos: size}, nil
}

// locate is where an HTML page takes the event tag: before the end tag of its
// head, or, where the page leaves that tag out, before the first thing that
// opens its body. It is -1 when the page is not HTML, is over headMax, already
// loads the tag, or ends with its head still open.
//
// Offsets are the tokenizer's, so text that only looks like a tag (in a script,
// a style, a title, a comment, an attribute) is never taken for one, and a head
// inside a template is not the page's. What keeps a head open and what opens
// the body follow the "in head" insertion mode (WHATWG HTML 13.2.6.4.4).
func locate(f io.Reader, name string) (int64, error) {
	r := io.LimitReader(f, headMax+1)
	if mime.TypeByExtension(filepath.Ext(name)) == "" {
		// No type from the name: HTML only if ServeContent sniffs it so.
		var b [512]byte
		n, err := io.ReadFull(r, b[:])
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, err
		}
		if !strings.HasPrefix(http.DetectContentType(b[:n]), "text/html") {
			return -1, nil
		}
		r = io.MultiReader(bytes.NewReader(b[:n]), r)
	}

	// A UTF-16 page announces itself with its byte order mark, which must stay
	// the first bytes; the tag is ASCII, so such a page is served as it is.
	br := bufio.NewReader(r)
	if b, _ := br.Peek(2); bytes.Equal(b, []byte("\xFE\xFF")) || bytes.Equal(b, []byte("\xFF\xFE")) {
		return -1, nil
	}

	z := html.NewTokenizer(br)
	at, pos := int64(-1), int64(0)
	head, raw, nest := true, false, 0 // the head is open; the text is an element's; template depth
	for {
		tt := z.Next()
		n := int64(len(z.Raw()))
		content := raw
		raw = false
		switch tt {
		case html.ErrorToken:
			if !errors.Is(z.Err(), io.EOF) {
				return 0, z.Err()
			}
			if pos > headMax {
				return -1, nil
			}
			return at, nil
		case html.TextToken:
			// A byte order mark is the encoding's, not text.
			text, skip := z.Text(), int64(0)
			if pos == 0 && bytes.HasPrefix(text, []byte("\xEF\xBB\xBF")) {
				text, skip = text[3:], 3
			}
			if head && nest == 0 && !content && len(bytes.Trim(text, " \t\n\f\r")) > 0 {
				at, head = pos+skip, false
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			b, more := z.TagName()
			tag := string(b)
			for tag == "script" && nest == 0 && more {
				var k, v []byte
				k, v, more = z.TagAttr()
				if string(k) == "src" && string(v) == eventSrc {
					return -1, nil
				}
			}
			if head && nest == 0 && !headHolds(tag) {
				at, head = pos, false
			}
			if tag == "template" {
				nest++
			}
			// Foreign content in a template can hold CDATA this scan does not
			// track, so such a page is served as it is.
			if nest > 0 && (tag == "svg" || tag == "math") {
				return -1, nil
			}
			// "/>" closes only a void element; these stay open either way.
			switch tag {
			case "title", "script", "style", "noscript", "noframes":
				raw = true
			}
		case html.EndTagToken:
			b, _ := z.TagName()
			switch tag := string(b); {
			case tag == "template" && nest > 0:
				nest--
			case head && nest == 0 && (tag == "head" || tag == "body" || tag == "html" || tag == "br"):
				at, head = pos, false
			}
		}
		pos += n
	}
}

// headHolds reports whether a start tag leaves the head open.
func headHolds(tag string) bool {
	switch tag {
	case "html", "head", "base", "basefont", "bgsound", "link", "meta", "title", "noscript", "noframes", "style", "script", "template":
		return true
	}
	return false
}

// splice is a page with the tag at offset at of its file, read from the file in
// place: no copy of the page is made, and a range reads only its own bytes.
type splice struct {
	f        io.ReadSeeker
	tag      []byte
	at, size int64 // the tag's offset in the file, the file's size
	pos      int64 // the position in the page
	fpos     int64 // the position in the file
}

func (s *splice) len() int64 { return s.size + int64(len(s.tag)) }

func (s *splice) Read(p []byte) (int, error) {
	tag := int64(len(s.tag))
	switch {
	case s.pos >= s.len():
		return 0, io.EOF
	case s.pos >= s.at && s.pos < s.at+tag:
		n := copy(p, s.tag[s.pos-s.at:])
		s.pos += int64(n)
		return n, nil
	}
	off, end := s.pos, s.at
	if s.pos >= s.at+tag {
		off, end = s.pos-tag, s.size
	}
	if off != s.fpos {
		if _, err := s.f.Seek(off, io.SeekStart); err != nil {
			return 0, err
		}
		s.fpos = off
	}
	n, err := s.f.Read(p[:min(int64(len(p)), end-off)])
	s.pos += int64(n)
	s.fpos += int64(n)
	if errors.Is(err, io.EOF) && s.pos < s.len() {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

func (s *splice) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += s.pos
	case io.SeekEnd:
		offset += s.len()
	default:
		return 0, errors.New("splice: invalid whence")
	}
	if offset < 0 {
		return 0, errors.New("splice: negative position")
	}
	s.pos = offset
	return offset, nil
}
