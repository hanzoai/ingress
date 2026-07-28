package loadbalancer

import (
	"errors"
	"net/http"
	"strconv"
)

// ErrNoAvailableServer is the single canonical "the pool cannot take this
// request" error, shared by every balancing strategy (wrr, p2c, hrw,
// leasttime). It means the pool is empty, every server is fenced, or health
// checks have marked them all down.
var ErrNoAvailableServer = errors.New("no available server")

// NoAvailableServerRetryAfter is the Retry-After (seconds) advertised with a
// saturated/empty pool. Kept short: the common cause is a rolling deploy or a
// brief health-check flap, where the pool is back within a second or two.
const NoAvailableServerRetryAfter = 2

// WriteNoAvailableServer writes the one canonical response for
// ErrNoAvailableServer: 503 Service Unavailable with Retry-After.
//
// Every strategy funnels through here so capacity exhaustion is reported
// identically no matter which balancer is configured. Two properties matter
// operationally and are the reason this is not just http.Error:
//
//   - Retry-After tells clients and probes this is transient back-pressure
//     rather than a hard failure, so they back off instead of hammering an
//     already-empty pool.
//   - Cache-Control: no-store stops any intermediary (CDN, browser) from
//     retaining the error. A cached capacity error outlives the outage that
//     produced it and turns a two-second deploy gap into a sticky one.
//
// The status is written before the body precisely so a saturated pool can
// never be mistaken for a healthy one: a status-code monitor must see 503.
func WriteNoAvailableServer(rw http.ResponseWriter) {
	h := rw.Header()
	h.Del("Content-Length")
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Retry-After", strconv.Itoa(NoAvailableServerRetryAfter))
	h.Set("Cache-Control", "no-store")

	rw.WriteHeader(http.StatusServiceUnavailable)
	_, _ = rw.Write([]byte(ErrNoAvailableServer.Error() + "\n"))
}
