package httputil

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ptypes "github.com/hanzoai/ingress-parser/types"
	"github.com/hanzoai/ingress/pkg/config/dynamic"
	"github.com/hanzoai/ingress/pkg/middlewares/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rollRetry is the Retry the edge puts in front of a backend that rolls: no
// status list, so only a connection-level failure retries; a short interval and
// a long timeout, so a request is held across the gap rather than failed.
func rollRetry() dynamic.Retry {
	return dynamic.Retry{
		Attempts:        1000,
		InitialInterval: ptypes.Duration(10 * time.Millisecond),
		Timeout:         ptypes.Duration(5 * time.Second),
	}
}

type retries struct{ n atomic.Int32 }

func (r *retries) Retried(*http.Request, int) { r.n.Add(1) }

// edge is the chain the service manager builds for one server: Retry, then
// retry.WrapHandler, then the reverse proxy over a real transport.
func edge(t *testing.T, backend string, l retry.Listener) http.Handler {
	t.Helper()

	target, err := url.Parse("http://" + backend)
	require.NoError(t, err)

	transport := &http.Transport{MaxIdleConnsPerHost: 10}
	t.Cleanup(transport.CloseIdleConnections)

	proxy := retry.WrapHandler(buildSingleHostProxy(target, false, false, 0, transport, nil))

	h, err := retry.New(t.Context(), proxy, rollRetry(), l, "roll")
	require.NoError(t, err)

	return h
}

// A POST whose bytes reached the backend is never sent again, however the
// backend fails afterwards. The backend here reads what arrives and closes the
// connection without answering, which is what a process killed mid-request does.
func TestRetryNeverResendsWhatTheBackendReceived(t *testing.T) {
	testCases := []struct {
		desc string
		read func(*bufio.Reader) error
	}{
		{
			desc: "backend read the whole request",
			read: func(r *bufio.Reader) error {
				req, err := http.ReadRequest(r)
				if err != nil {
					return err
				}
				_, err = io.Copy(io.Discard, req.Body)
				return err
			},
		},
		{
			desc: "backend read only the request line",
			read: func(r *bufio.Reader) error {
				_, err := r.ReadString('\n')
				return err
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			t.Cleanup(func() { _ = ln.Close() })

			var received atomic.Int32
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					conn, err := ln.Accept()
					if err != nil {
						return
					}
					if err := test.read(bufio.NewReader(conn)); err == nil {
						received.Add(1)
					}
					_ = conn.Close()
				}
			}()

			l := &retries{}
			h := edge(t, ln.Addr().String(), l)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "http://api.example/v1/chat/completions",
				strings.NewReader(`{"model":"zen","messages":[{"role":"user","content":"hi"}]}`))
			h.ServeHTTP(rec, req)

			// Give a duplicate time to arrive before counting.
			time.Sleep(200 * time.Millisecond)
			_ = ln.Close()
			wg.Wait()

			assert.Equal(t, int32(1), received.Load(), "the backend must see the POST exactly once")
			assert.Equal(t, int32(0), l.n.Load(), "no retry once bytes reached the backend")
			assert.Equal(t, http.StatusBadGateway, rec.Code)
		})
	}
}

// A POST that reached no backend — nothing is listening — is held and retried
// until one listens, and then lands exactly once.
func TestRetryHoldsAPostUntilTheBackendListens(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close()) // the port refuses until the backend binds it

	var received atomic.Int32
	srv := &http.Server{Handler: http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		if string(body) == "once" {
			received.Add(1)
		}
		rw.WriteHeader(http.StatusOK)
	})}

	go func() {
		time.Sleep(300 * time.Millisecond)
		up, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		_ = srv.Serve(up)
	}()
	t.Cleanup(func() { _ = srv.Close() })

	l := &retries{}
	h := edge(t, addr, l)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://api.example/v1/chat/completions", strings.NewReader("once"))
	start := time.Now()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int32(1), received.Load(), "the held POST lands exactly once")
	assert.Positive(t, l.n.Load(), "refused connections are retried")
	assert.GreaterOrEqual(t, time.Since(start), 300*time.Millisecond)
}
