package staticfiles

import (
	"context"
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

	"github.com/hanzoai/ingress/v3/pkg/config/dynamic"
	"github.com/hanzoai/ingress/v3/pkg/middlewares"
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
	rootPath             string
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

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("invalid root path: %w", err)
	}

	if _, err := os.Stat(absRoot); os.IsNotExist(err) {
		if err := os.MkdirAll(absRoot, 0755); err != nil {
			return nil, fmt.Errorf("failed to create root directory: %w", err)
		}
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
		root:                 http.Dir(absRoot),
		rootPath:             absRoot,
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

func (h *staticFiles) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
	}

	f, err := h.root.Open(upath)
	if err != nil {
		if os.IsNotExist(err) {
			if h.spaMode {
				h.serveFile(w, r, filepath.Join(h.rootPath, h.spaIndex))
				return
			}
			if h.errorPage404 != "" {
				w.WriteHeader(h.notFoundResponseCode)
				h.serveFile(w, r, filepath.Join(h.rootPath, h.errorPage404))
				return
			}
			http.NotFound(w, r)
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
			indexFile, err := h.root.Open(indexPath)
			if err == nil {
				indexFile.Close()
				localRedirect(w, r, indexPath)
				return
			}
		}

		if !h.enableDirListing {
			if h.errorPage404 != "" {
				w.WriteHeader(h.notFoundResponseCode)
				h.serveFile(w, r, filepath.Join(h.rootPath, h.errorPage404))
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

	if maxAge, ok := h.cacheControl[ext]; ok {
		w.Header().Set("Cache-Control", maxAge)
	} else if maxAge, ok := h.cacheControl["*"]; ok {
		w.Header().Set("Cache-Control", maxAge)
	} else {
		w.Header().Set("Cache-Control", "max-age=86400")
	}

	w.Header().Set("Last-Modified", d.ModTime().UTC().Format(http.TimeFormat))
}

func (h *staticFiles) serveFile(w http.ResponseWriter, r *http.Request, filePath string) {
	f, err := os.Open(filePath)
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
