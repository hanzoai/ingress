package loadbalancer

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A saturated/empty pool must be reported as a real error status. A 200 here
// passes every status-code-based monitor and is exactly how a broken fleet
// reports itself healthy.
func TestWriteNoAvailableServer_StatusIs503NotOK(t *testing.T) {
	rw := httptest.NewRecorder()

	WriteNoAvailableServer(rw)

	assert.Equal(t, http.StatusServiceUnavailable, rw.Code)
	assert.NotEqual(t, http.StatusOK, rw.Code)
	assert.Equal(t, "no available server\n", rw.Body.String())
}

// Retry-After marks this as transient back-pressure so clients and probes back
// off instead of hammering an already-empty pool.
func TestWriteNoAvailableServer_RetryAfter(t *testing.T) {
	rw := httptest.NewRecorder()

	WriteNoAvailableServer(rw)

	assert.Equal(t, "2", rw.Header().Get("Retry-After"))
}

// A cached capacity error outlives the outage that produced it, turning a
// two-second deploy gap into a sticky one.
func TestWriteNoAvailableServer_NotCacheable(t *testing.T) {
	rw := httptest.NewRecorder()

	WriteNoAvailableServer(rw)

	assert.Equal(t, "no-store", rw.Header().Get("Cache-Control"))
	assert.Equal(t, "text/plain; charset=utf-8", rw.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", rw.Header().Get("X-Content-Type-Options"))
}

// A stale Content-Length from an earlier handler must not survive onto the
// error body, or the response is truncated/corrupt on the wire.
func TestWriteNoAvailableServer_ClearsStaleContentLength(t *testing.T) {
	rw := httptest.NewRecorder()
	rw.Header().Set("Content-Length", "99999")

	WriteNoAvailableServer(rw)

	assert.Empty(t, rw.Header().Get("Content-Length"))
}
