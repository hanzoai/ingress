package staticfiles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	s3 "github.com/hanzos3/go-sdk"
	"github.com/hanzos3/go-sdk/pkg/credentials"
)

const (
	// metadataTimeout bounds a single object-store metadata call (stat, list).
	metadataTimeout = 10 * time.Second
	// objectServeTimeout bounds the lifetime of one streamed object read.
	objectServeTimeout = 5 * time.Minute
)

// objectInfo is the metadata the file server needs about one object.
// It satisfies fs.FileInfo so it flows through the existing handler unchanged,
// and exposes ETag so the handler can set a strong validator.
type objectInfo struct {
	key     string
	size    int64
	modTime time.Time
	etag    string
	isDir   bool
}

func (i objectInfo) Name() string { return path.Base(i.key) }
func (i objectInfo) Size() int64  { return i.size }
func (i objectInfo) Mode() fs.FileMode {
	if i.isDir {
		return fs.ModeDir | 0o555
	}
	return 0o444
}
func (i objectInfo) ModTime() time.Time { return i.modTime }
func (i objectInfo) IsDir() bool        { return i.isDir }
func (i objectInfo) Sys() any           { return nil }
func (i objectInfo) ETag() string       { return i.etag }

// etagger is implemented by fs.FileInfo values that carry a strong validator.
// Local os.FileInfo does not implement it, so the ETag header is set only for
// the object-store origin.
type etagger interface{ ETag() string }

// readSeekCloser is the streaming, seekable body of one object. The object
// store's implementation reads ranges lazily so memory stays bounded — the
// whole object is never buffered.
type readSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

// objectStore is the minimal object-store surface the S3 file system needs.
// It is bucket-scoped. Splitting it from the http.FileSystem keeps the transport
// (real S3 vs. a test double) orthogonal to the serving logic.
type objectStore interface {
	// stat returns object metadata, or fs.ErrNotExist when the key is absent.
	stat(ctx context.Context, key string) (objectInfo, error)
	// open returns a bounded, seekable stream for the object.
	open(ctx context.Context, key string) (readSeekCloser, error)
	// list returns the immediate children of a prefix (one directory level).
	list(ctx context.Context, prefix string) ([]objectInfo, error)
}

// s3FS is an http.FileSystem backed by an object store under a fixed prefix.
type s3FS struct {
	store  objectStore
	prefix string
}

// parseObjectRoot parses an "s3://bucket/prefix" root into its parts.
func parseObjectRoot(root string) (bucket, prefix string, ok bool) {
	u, err := url.Parse(root)
	if err != nil || u.Scheme != "s3" || u.Host == "" {
		return "", "", false
	}
	return u.Host, strings.Trim(u.Path, "/"), true
}

// cleanRel collapses ".", ".." and duplicate slashes against an absolute root.
// path.Clean on an absolute path can never escape above "/", so the result is
// always safe to join under the middleware's prefix — a request can never read
// outside its own site.
func cleanRel(name string) string {
	return strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(name, "/")), "/")
}

// keyFor maps a request path to an object key inside the configured prefix.
func (s *s3FS) keyFor(name string) string {
	rel := cleanRel(name)
	switch {
	case s.prefix == "":
		return rel
	case rel == "":
		return s.prefix
	default:
		return s.prefix + "/" + rel
	}
}

// Open implements http.FileSystem with a background context.
func (s *s3FS) Open(name string) (http.File, error) {
	return s.openCtx(context.Background(), name)
}

// openCtx resolves a path bound to ctx, so a client disconnect cancels the
// object-store reads. Directories (root and any trailing-slash path) resolve to
// a lazily-listed directory; everything else resolves to an object, or
// fs.ErrNotExist so the handler can apply its SPA / 404 policy.
func (s *s3FS) openCtx(ctx context.Context, name string) (http.File, error) {
	if name == "" {
		name = "/"
	}

	key := s.keyFor(name)

	if name == "/" || strings.HasSuffix(name, "/") {
		return &s3Dir{store: s.store, key: key}, nil
	}

	sctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	info, err := s.store.stat(sctx, key)
	cancel()
	if err != nil {
		return nil, mapOpenErr(name, err)
	}

	// The stream lives until Close; its context is a child of the request's,
	// with an objectServeTimeout backstop, and is cancelled in Close — so a
	// disconnected or slow client frees the upstream connection promptly.
	octx, ocancel := context.WithTimeout(ctx, objectServeTimeout)
	rc, err := s.store.open(octx, key)
	if err != nil {
		ocancel()
		return nil, mapOpenErr(name, err)
	}
	return &s3File{rc: rc, info: info, cancel: ocancel}, nil
}

// mapOpenErr normalizes a not-found into os.ErrNotExist (so os.IsNotExist drives
// the handler's SPA / 404 policy) and passes every other error through so the
// handler can tell access-denied from an object-store outage.
func mapOpenErr(name string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return &os.PathError{Op: "open", Path: name, Err: os.ErrNotExist}
	}
	return err
}

// s3File is one object exposed as an http.File. Read/Seek delegate to the
// store's bounded stream, so http.ServeContent (which seeks to size, then
// streams) never buffers the whole object.
type s3File struct {
	rc     readSeekCloser
	info   objectInfo
	cancel context.CancelFunc
}

func (f *s3File) Read(p []byte) (int, error)                { return f.rc.Read(p) }
func (f *s3File) Seek(off int64, whence int) (int64, error) { return f.rc.Seek(off, whence) }
func (f *s3File) Stat() (fs.FileInfo, error)                { return f.info, nil }
func (f *s3File) Readdir(int) ([]fs.FileInfo, error) {
	return nil, &os.PathError{Op: "readdir", Path: f.info.key, Err: os.ErrInvalid}
}
func (f *s3File) Close() error {
	if f.cancel != nil {
		f.cancel()
	}
	return f.rc.Close()
}

// s3Dir is a directory (prefix) exposed as an http.File. Only Stat and Readdir
// are meaningful; Read/Seek exist to satisfy http.File and are never reached
// because the handler branches on IsDir first.
type s3Dir struct {
	store objectStore
	key   string
}

func (d *s3Dir) Read([]byte) (int, error) {
	return 0, &os.PathError{Op: "read", Path: d.key, Err: os.ErrInvalid}
}
func (d *s3Dir) Seek(int64, int) (int64, error) {
	return 0, &os.PathError{Op: "seek", Path: d.key, Err: os.ErrInvalid}
}
func (d *s3Dir) Close() error { return nil }
func (d *s3Dir) Stat() (fs.FileInfo, error) {
	return objectInfo{key: d.key, isDir: true}, nil
}
func (d *s3Dir) Readdir(int) ([]fs.FileInfo, error) {
	prefix := d.key
	if prefix != "" {
		prefix += "/"
	}
	ctx, cancel := context.WithTimeout(context.Background(), metadataTimeout)
	defer cancel()
	infos, err := d.store.list(ctx, prefix)
	if err != nil {
		return nil, err
	}
	out := make([]fs.FileInfo, 0, len(infos))
	for _, i := range infos {
		out = append(out, i)
	}
	return out, nil
}

// minioStore is the production object store over hanzos3/go-sdk. It is the one
// object-store implementation shared with the standalone static server.
type minioStore struct {
	client *s3.Client
	bucket string
}

func (m *minioStore) stat(ctx context.Context, key string) (objectInfo, error) {
	oi, err := m.client.StatObject(ctx, m.bucket, key, s3.StatObjectOptions{})
	if err != nil {
		return objectInfo{}, classifyStoreErr(err)
	}
	return objectInfo{key: key, size: oi.Size, modTime: oi.LastModified, etag: oi.ETag}, nil
}

func (m *minioStore) open(ctx context.Context, key string) (readSeekCloser, error) {
	obj, err := m.client.GetObject(ctx, m.bucket, key, s3.GetObjectOptions{})
	if err != nil {
		return nil, classifyStoreErr(err)
	}
	return obj, nil
}

func (m *minioStore) list(ctx context.Context, prefix string) ([]objectInfo, error) {
	self := strings.TrimSuffix(prefix, "/")
	var out []objectInfo
	for oi := range m.client.ListObjects(ctx, m.bucket, s3.ListObjectsOptions{Prefix: prefix}) {
		if oi.Err != nil {
			return nil, oi.Err
		}
		isDir := strings.HasSuffix(oi.Key, "/")
		key := strings.TrimSuffix(oi.Key, "/")
		if key == self {
			continue
		}
		out = append(out, objectInfo{key: key, size: oi.Size, modTime: oi.LastModified, etag: oi.ETag, isDir: isDir})
	}
	return out, nil
}

// errObjectStoreUnavailable marks an object-store transport/availability
// failure (down, timeout, 5xx) so the handler answers 502 rather than
// mislabeling an outage as an authorization failure.
var errObjectStoreUnavailable = errors.New("object store unavailable")

// classifyStoreErr maps an object-store error to the handler's response policy:
// missing -> fs.ErrNotExist (404 / SPA fallback); access denied ->
// fs.ErrPermission (403); anything else -> errObjectStoreUnavailable (502).
func classifyStoreErr(err error) error {
	resp := s3.ToErrorResponse(err)
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.Code == "NoSuchKey" || resp.Code == "NoSuchBucket":
		return fs.ErrNotExist
	case resp.StatusCode == http.StatusForbidden || resp.Code == "AccessDenied":
		return fs.ErrPermission
	default:
		return fmt.Errorf("%w: %w", errObjectStoreUnavailable, err)
	}
}

// newObjectFS builds an object-store file system for an "s3://bucket/prefix"
// root. The object store (endpoint, region, credentials) is defined ONLY by the
// ingress environment — one shared store for the whole fleet — so a Middleware
// CR can neither point the ingress credential at another host nor leak a secret
// into the dynamic configuration plane. It fails closed: an empty prefix (which
// would expose the whole shared bucket), a missing endpoint, or missing
// credentials all refuse the build.
func newObjectFS(root string) (*s3FS, error) {
	bucket, prefix, ok := parseObjectRoot(root)
	if !ok {
		return nil, fmt.Errorf("invalid s3 root %q (want s3://bucket/prefix)", root)
	}
	if prefix == "" {
		return nil, fmt.Errorf("s3 root %q needs a non-empty prefix (s3://bucket/prefix); serving a whole bucket would expose every other site", root)
	}

	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		return nil, fmt.Errorf("s3 root %q needs S3_ENDPOINT in the ingress environment", root)
	}

	region := os.Getenv("S3_REGION")
	if region == "" {
		region = "us-east-1"
	}

	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("s3 root %q needs credentials in AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY", root)
	}

	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	client, err := s3.New(endpoint, &s3.Options{
		Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:       os.Getenv("S3_USE_SSL") == "true",
		Region:       region,
		BucketLookup: s3.BucketLookupPath,
	})
	if err != nil {
		return nil, fmt.Errorf("s3 client: %w", err)
	}

	return &s3FS{store: &minioStore{client: client, bucket: bucket}, prefix: prefix}, nil
}
