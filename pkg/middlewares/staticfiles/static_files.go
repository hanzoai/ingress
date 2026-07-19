package staticfiles

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
	"github.com/hanzoai/ingress/pkg/middlewares"
)

const typeName = "StaticFiles"

type dirEntry struct {
	Name    string
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
	IsDir   bool
}

type staticFiles struct {
	root                 http.FileSystem
	enableDirListing     bool
	indexFiles           []string
	spaMode              bool
	spaIndex             string
	errorPage404         string
	cacheControl         map[string]string
	notFoundResponseCode int
	name                 string
	next                 http.Handler
}

// New creates a new static files middleware.
func New(ctx context.Context, next http.Handler, config dynamic.StaticFiles, name string) (http.Handler, error) {
	logger := middlewares.GetLogger(ctx, name, typeName)
	logger.Debug().Msg("Creating middleware")

	root := config.Root
	if root == "" {
		root = "."
	}

	// The origin is a union: a local directory or an object store, selected by
	// the Root form. An "s3://bucket/prefix" Root serves from the object store;
	// anything else is a local path. Both are served through the same handler
	// via http.FileSystem, so only the origin differs.
	var rootFS http.FileSystem
	if _, _, ok := parseObjectRoot(root); ok {
		fsys, err := newObjectFS(root)
		if err != nil {
			return nil, err
		}
		rootFS = fsys
	} else {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("invalid root path: %w", err)
		}
		if _, err := os.Stat(absRoot); os.IsNotExist(err) {
			if err := os.MkdirAll(absRoot, 0755); err != nil {
				return nil, fmt.Errorf("failed to create root directory: %w", err)
			}
		}
		rootFS = http.Dir(absRoot)
	}

	indexFiles := config.IndexFiles
	if len(indexFiles) == 0 {
		indexFiles = []string{"index.html", "index.htm"}
	}

	spaIndex := config.SPAIndex
	if spaIndex == "" {
		spaIndex = "index.html"
	}

	notFoundResponseCode := http.StatusNotFound
	if config.ErrorPage404 != "" {
		notFoundResponseCode = http.StatusOK
	}

	return &staticFiles{
		root:                 rootFS,
		enableDirListing:     config.EnableDirectoryListing,
		indexFiles:           indexFiles,
		spaMode:              config.SPAMode,
		spaIndex:             spaIndex,
		errorPage404:         config.ErrorPage404,
		cacheControl:         config.CacheControl,
		notFoundResponseCode: notFoundResponseCode,
		name:                 name,
		next:                 next,
	}, nil
}

func (h *staticFiles) GetTracingInformation() (string, string) {
	return h.name, typeName
}

// contextFS is an http.FileSystem whose opens can be bound to a request context.
type contextFS interface {
	openCtx(ctx context.Context, name string) (http.File, error)
}

// open resolves a path against the root, binding object-store reads to ctx when
// the origin supports it so a client disconnect cancels the upstream read. The
// local origin (http.Dir) has no context to bind and is opened directly.
func (h *staticFiles) open(ctx context.Context, name string) (http.File, error) {
	if cf, ok := h.root.(contextFS); ok {
		return cf.openCtx(ctx, name)
	}
	return h.root.Open(name)
}

// looksLikeAsset reports whether a not-found path should return 404 rather than
// the SPA shell: true for a concrete file (a non-HTML extension), false for an
// extensionless client-route navigation. This stops a missing content-hashed
// asset from being masked by a 200 index.html.
func looksLikeAsset(p string) bool {
	ext := strings.ToLower(path.Ext(p))
	return ext != "" && ext != ".html" && ext != ".htm"
}

func (h *staticFiles) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
	}

	f, err := h.open(r.Context(), upath)
	if err != nil {
		if os.IsNotExist(err) {
			if h.spaMode && !looksLikeAsset(upath) {
				h.serveFile(w, r, h.spaIndex)
				return
			}
			if h.errorPage404 != "" {
				w.WriteHeader(h.notFoundResponseCode)
				h.serveFile(w, r, h.errorPage404)
				return
			}
			http.NotFound(w, r)
			return
		}
		// An object-store outage is an availability failure, not authorization:
		// answer 502, and keep access-denied / local errors as 403.
		if errors.Is(err, errObjectStoreUnavailable) {
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
			return
		}
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	defer f.Close()

	d, err := f.Stat()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if d.IsDir() {
		url := r.URL.Path
		if len(url) == 0 || url[len(url)-1] != '/' {
			localRedirect(w, r, url+"/")
			return
		}

		for _, index := range h.indexFiles {
			indexPath := path.Join(upath, index)
			indexFile, err := h.open(r.Context(), indexPath)
			if err == nil {
				indexFile.Close()
				localRedirect(w, r, indexPath)
				return
			}
		}

		if !h.enableDirListing {
			if h.errorPage404 != "" {
				w.WriteHeader(h.notFoundResponseCode)
				h.serveFile(w, r, h.errorPage404)
				return
			}
			http.NotFound(w, r)
			return
		}

		h.serveDirectoryListing(w, r, f)
		return
	}

	h.setCacheHeaders(w, d)

	name := d.Name()
	ext := filepath.Ext(name)
	contentType := mime.TypeByExtension(ext)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	http.ServeContent(w, r, d.Name(), d.ModTime(), f.(io.ReadSeeker))
}

func (h *staticFiles) serveDirectoryListing(w http.ResponseWriter, r *http.Request, f http.File) {
	dirs, err := f.Readdir(-1)
	if err != nil {
		http.Error(w, "Error reading directory", http.StatusInternalServerError)
		return
	}

	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i].IsDir() && !dirs[j].IsDir() {
			return true
		}
		if !dirs[i].IsDir() && dirs[j].IsDir() {
			return false
		}
		return dirs[i].Name() < dirs[j].Name()
	})

	entries := make([]dirEntry, len(dirs))
	for i, entry := range dirs {
		entries[i] = dirEntry{
			Name:    entry.Name(),
			Size:    entry.Size(),
			Mode:    entry.Mode(),
			ModTime: entry.ModTime(),
			IsDir:   entry.IsDir(),
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tmpl := template.Must(template.New("dirlist").Parse(dirListingTemplate))

	data := struct {
		Path  string
		Files []dirEntry
	}{
		Path:  r.URL.Path,
		Files: entries,
	}

	if err = tmpl.Execute(w, data); err != nil {
		http.Error(w, "Error rendering directory listing", http.StatusInternalServerError)
	}
}

func (h *staticFiles) setCacheHeaders(w http.ResponseWriter, d fs.FileInfo) {
	ext := filepath.Ext(d.Name())

	switch {
	case cacheControlHas(h.cacheControl, ext):
		w.Header().Set("Cache-Control", h.cacheControl[ext])
	case cacheControlHas(h.cacheControl, "*"):
		w.Header().Set("Cache-Control", h.cacheControl["*"])
	case ext == ".html" || ext == ".htm":
		// The shell references content-hashed assets; served stale it would pin
		// clients to an old asset graph. Revalidate every time (cheap: the ETag
		// / Last-Modified below answers 304 when unchanged).
		w.Header().Set("Cache-Control", "no-cache")
	default:
		w.Header().Set("Cache-Control", "max-age=86400")
	}

	// This plane serves third-party bundles; never let the browser MIME-sniff.
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// A strong validator lets http.ServeContent answer conditional requests
	// (If-None-Match). Only the object-store origin carries one; the local
	// origin is unchanged.
	if e, ok := d.(etagger); ok {
		if tag := e.ETag(); tag != "" {
			if !strings.HasPrefix(tag, `"`) {
				tag = `"` + tag + `"`
			}
			w.Header().Set("ETag", tag)
		}
	}

	w.Header().Set("Last-Modified", d.ModTime().UTC().Format(http.TimeFormat))
}

func cacheControlHas(m map[string]string, key string) bool {
	_, ok := m[key]
	return ok
}

// serveFile serves a single file (SPA index or 404 page) named relative to the
// root, through the same origin as everything else — local disk or object store.
func (h *staticFiles) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	f, err := h.open(r.Context(), "/"+strings.TrimPrefix(name, "/"))
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	d, err := f.Stat()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.setCacheHeaders(w, d)

	ext := filepath.Ext(d.Name())
	contentType := mime.TypeByExtension(ext)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	http.ServeContent(w, r, d.Name(), d.ModTime(), f)
}

func localRedirect(w http.ResponseWriter, r *http.Request, newPath string) {
	if q := r.URL.RawQuery; q != "" {
		newPath += "?" + q
	}
	w.Header().Set("Location", newPath)
	w.WriteHeader(http.StatusMovedPermanently)
}

const dirListingTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Index of {{.Path}}</title>
    <style>
        body { font-family: sans-serif; margin: 2em; }
        table { border-collapse: collapse; width: 100%; }
        th, td { text-align: left; padding: 8px; }
        tr:nth-child(even) { background-color: #f2f2f2; }
        th { background-color: #333; color: white; }
        a { text-decoration: none; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <h1>Index of {{.Path}}</h1>
    <table>
        <tr>
            <th>Name</th>
            <th>Size</th>
            <th>Modified</th>
        </tr>
        {{if ne .Path "/"}}
        <tr>
            <td><a href="../">../</a></td>
            <td>-</td>
            <td>-</td>
        </tr>
        {{end}}
        {{range .Files}}
        <tr>
            <td><a href="{{.Name}}{{if .IsDir}}/{{end}}">{{.Name}}{{if .IsDir}}/{{end}}</a></td>
            <td>{{if .IsDir}}-{{else}}{{.Size}} bytes{{end}}</td>
            <td>{{.ModTime.Format "2006-01-02 15:04:05"}}</td>
        </tr>
        {{end}}
    </table>
</body>
</html>`
