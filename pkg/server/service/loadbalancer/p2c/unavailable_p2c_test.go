package p2c

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// An empty pool is capacity exhaustion, not success. Returning 200 here would
// pass every status-code-based monitor while serving nothing.
func TestBalancerNoServersReturns503WithRetryAfter(t *testing.T) {
	balancer := New(nil, false)

	recorder := httptest.NewRecorder()
	balancer.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://localhost/", nil))

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, "2", recorder.Header().Get("Retry-After"))
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, "no available server\n", recorder.Body.String())
}
