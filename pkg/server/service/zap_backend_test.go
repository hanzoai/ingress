package service

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	zaphttp "github.com/zap-proto/http"
)

// TestZapBackendRoundTripper_RoutesAllowlistedHostToZap stands up a real
// ZAP-HTTP server (a fasthttp handler, per zap-proto/http v0.1) and marks its
// host:port as a ZAP backend in the allowlist. It exercises the full
// *http.Request -> *fasthttp.Request -> Transport.Do -> *fasthttp.Response ->
// *http.Response translation over three round trips — a GET, a body-carrying
// POST echo, and a non-200 status — asserting method, path, body, headers,
// status, and trailers survive. A second backend not in the allowlist is
// served over plain HTTP and must keep using the wrapped transport.
func TestZapBackendRoundTripper_RoutesAllowlistedHostToZap(t *testing.T) {
	// 1. Spin up the ZAP-HTTP backend. Handler is a fasthttp.RequestHandler.
	zapLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer zapLn.Close()
	zapAddr := zapLn.Addr().String()

	zapHandler := func(ctx *fasthttp.RequestCtx) {
		ctx.Response.Header.Set("X-Backend", "zap")
		switch {
		case string(ctx.Path()) == "/healthz":
			ctx.SetBodyString("zap-ok")
		case string(ctx.Path()) == "/echo" && string(ctx.Method()) == fasthttp.MethodPost:
			// Echo the request body back and report its length, proving the
			// request body and method crossed the wire intact. A response
			// trailer exercises the trailer-lifting path.
			ctx.Response.Header.Set("X-Echo-Len", strconv.Itoa(len(ctx.PostBody())))
			_ = ctx.Response.Header.AddTrailer("X-Checksum")
			ctx.Response.Header.Set("X-Checksum", "deadbeef")
			ctx.Write(ctx.PostBody()) //nolint:errcheck
		case string(ctx.Path()) == "/teapot":
			ctx.SetStatusCode(fasthttp.StatusTeapot)
			ctx.SetBodyString("short and stout")
		default:
			ctx.SetStatusCode(fasthttp.StatusNotFound)
			ctx.SetBodyString("no route")
		}
	}
	zapSrv := &zaphttp.Server{Handler: zapHandler, ReadTimeout: 5 * time.Second}
	go func() { _ = zapSrv.Serve(zapLn) }()
	defer zapSrv.Close()

	// 2. Spin up the plain-HTTP backend (must not be touched by ZAP).
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "http")
		_, _ = io.WriteString(w, "http-ok")
	}))
	defer httpSrv.Close()
	httpAddr := strings.TrimPrefix(httpSrv.URL, "http://")

	// 3. Build the wrapper directly. We bypass env-var parsing so the test
	//    does not race with other tests in the same package.
	rt := &zapBackendRoundTripper{
		next:      http.DefaultTransport,
		allowlist: map[string]struct{}{zapAddr: {}},
	}

	// 4. Allowlisted host, GET: reaches the ZAP backend; header + body + status.
	req, err := http.NewRequest(http.MethodGet, "http://"+zapAddr+"/healthz", nil)
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("zap GET roundtrip: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if got := string(body); got != "zap-ok" {
		t.Fatalf("zap GET body = %q, want zap-ok", got)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("zap GET status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Backend"); got != "zap" {
		t.Fatalf("zap GET header = %q, want zap", got)
	}

	// 5. Allowlisted host, POST with a body: request body echoes back, and a
	//    response trailer is lifted into Response.Trailer (not Response.Header).
	payload := []byte("the quick brown fox\x00\x01\x02")
	postReq, err := http.NewRequest(http.MethodPost, "http://"+zapAddr+"/echo", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new post req: %v", err)
	}
	postResp, err := rt.RoundTrip(postReq)
	if err != nil {
		t.Fatalf("zap POST roundtrip: %v", err)
	}
	postBody, _ := io.ReadAll(postResp.Body)
	postResp.Body.Close()
	if !bytes.Equal(postBody, payload) {
		t.Fatalf("zap POST echo body = %q, want %q", postBody, payload)
	}
	if got := postResp.Header.Get("X-Echo-Len"); got != strconv.Itoa(len(payload)) {
		t.Fatalf("zap POST X-Echo-Len = %q, want %d", got, len(payload))
	}
	if postResp.ContentLength != int64(len(payload)) {
		t.Fatalf("zap POST ContentLength = %d, want %d", postResp.ContentLength, len(payload))
	}
	if got := postResp.Trailer.Get("X-Checksum"); got != "deadbeef" {
		t.Fatalf("zap POST trailer X-Checksum = %q, want deadbeef", got)
	}
	if got := postResp.Header.Get("X-Checksum"); got != "" {
		t.Fatalf("zap POST leaked trailer into Header: X-Checksum = %q, want empty", got)
	}

	// 6. Allowlisted host, non-200: status and body propagate.
	teaReq, err := http.NewRequest(http.MethodGet, "http://"+zapAddr+"/teapot", nil)
	if err != nil {
		t.Fatalf("new teapot req: %v", err)
	}
	teaResp, err := rt.RoundTrip(teaReq)
	if err != nil {
		t.Fatalf("zap teapot roundtrip: %v", err)
	}
	teaBody, _ := io.ReadAll(teaResp.Body)
	teaResp.Body.Close()
	if teaResp.StatusCode != http.StatusTeapot {
		t.Fatalf("zap teapot status = %d, want 418", teaResp.StatusCode)
	}
	if got := string(teaBody); got != "short and stout" {
		t.Fatalf("zap teapot body = %q, want short and stout", got)
	}

	// 7. Non-allowlisted host: must fall through to the wrapped transport.
	req2, err := http.NewRequest(http.MethodGet, "http://"+httpAddr+"/anything", nil)
	if err != nil {
		t.Fatalf("new req2: %v", err)
	}
	resp2, err := rt.RoundTrip(req2)
	if err != nil {
		t.Fatalf("http roundtrip: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if got := string(body2); got != "http-ok" {
		t.Fatalf("http body = %q, want http-ok", got)
	}
	if got := resp2.Header.Get("X-Backend"); got != "http" {
		t.Fatalf("http header = %q, want http", got)
	}
}

// TestNewZapBackendRoundTripper_EmptyEnvReturnsNext: when no env var is
// set, the constructor returns the wrapped transport unchanged. This is
// the additive-only guarantee — opt-out by default.
func TestNewZapBackendRoundTripper_EmptyEnvReturnsNext(t *testing.T) {
	t.Setenv(zapBackendEnv, "")
	next := http.DefaultTransport
	got := newZapBackendRoundTripper(next)
	if got != next {
		t.Fatalf("with empty env, expected wrapped == next; got wrapper")
	}
}

// TestParseZapBackendList exercises the comma-separated env parser.
func TestParseZapBackendList(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a:1", []string{"a:1"}},
		{"a:1,b:2", []string{"a:1", "b:2"}},
		{" a:1 , b:2 ,, ", []string{"a:1", "b:2"}},
	}
	for _, tt := range tests {
		got := parseZapBackendList(tt.in)
		if len(got) != len(tt.want) {
			t.Fatalf("parse(%q) = %v, want %v", tt.in, got, tt.want)
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Fatalf("parse(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
			}
		}
	}
}
