package geoblock

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
)

// fakeResolver returns a fixed country for any IP — used to drive the
// middleware without needing a real MaxMind DB on disk.
type fakeResolver struct {
	country string
}

func (f fakeResolver) Lookup(net.IP) string { return f.country }
func (f fakeResolver) Close() error         { return nil }

func newTestMiddleware(t *testing.T, cfg dynamic.GeoBlock, res resolver) *geoBlock {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo the country header so tests can assert injection happened.
		hdr := cfg.HeaderName
		if hdr == "" {
			hdr = defaultHeader
		}
		if c := r.Header.Get(hdr); c != "" {
			w.Header().Set("Echo-Country", c)
		}
		w.WriteHeader(http.StatusOK)
	})
	mw, err := New(context.Background(), next, cfg, "test-geoblock")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	g := mw.(*geoBlock)
	g.resolver = res
	return g
}

func TestHeaderInjection_DefaultName(t *testing.T) {
	g := newTestMiddleware(t, dynamic.GeoBlock{}, fakeResolver{country: "US"})

	req := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	req.RemoteAddr = "8.8.8.8:1"
	rw := httptest.NewRecorder()
	g.ServeHTTP(rw, req)

	if got := rw.Result().Header.Get("Echo-Country"); got != "US" {
		t.Fatalf("expected echoed country US, got %q", got)
	}
}

func TestHeaderInjection_CustomName(t *testing.T) {
	g := newTestMiddleware(t, dynamic.GeoBlock{HeaderName: "X-Country-ISO"}, fakeResolver{country: "JP"})

	req := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	req.RemoteAddr = "8.8.8.8:1"
	rw := httptest.NewRecorder()
	g.ServeHTTP(rw, req)

	if got := rw.Result().Header.Get("Echo-Country"); got != "JP" {
		t.Fatalf("expected echoed country JP, got %q", got)
	}
}

func TestBlockMode_Blocks(t *testing.T) {
	g := newTestMiddleware(t, dynamic.GeoBlock{
		BlockCountries: []string{"CU", "IR", "KP", "SY"},
	}, fakeResolver{country: "IR"})

	req := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	req.RemoteAddr = "8.8.8.8:1"
	rw := httptest.NewRecorder()
	g.ServeHTTP(rw, req)

	if rw.Result().StatusCode != http.StatusUnavailableForLegalReasons {
		t.Fatalf("expected 451, got %d", rw.Result().StatusCode)
	}
	if reason := rw.Result().Header.Get("X-Block-Reason"); reason != "geo:IR" {
		t.Fatalf("expected X-Block-Reason geo:IR, got %q", reason)
	}
}

func TestBlockMode_PassesThrough(t *testing.T) {
	g := newTestMiddleware(t, dynamic.GeoBlock{
		BlockCountries: []string{"CU", "IR", "KP", "SY"},
	}, fakeResolver{country: "US"})

	req := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	req.RemoteAddr = "8.8.8.8:1"
	rw := httptest.NewRecorder()
	g.ServeHTTP(rw, req)

	if rw.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", rw.Result().StatusCode)
	}
	if got := rw.Result().Header.Get("Echo-Country"); got != "US" {
		t.Fatalf("expected country header US, got %q", got)
	}
}

func TestAllowMode_OnlyAllowed(t *testing.T) {
	cfg := dynamic.GeoBlock{
		AllowCountries: []string{"US", "CA", "GB"},
	}
	for _, tc := range []struct {
		country string
		ok      bool
	}{
		{"US", true},
		{"CA", true},
		{"GB", true},
		{"FR", false},
		{"IR", false},
	} {
		g := newTestMiddleware(t, cfg, fakeResolver{country: tc.country})
		req := httptest.NewRequest(http.MethodGet, "http://x/", nil)
		req.RemoteAddr = "8.8.8.8:1"
		rw := httptest.NewRecorder()
		g.ServeHTTP(rw, req)
		got := rw.Result().StatusCode
		if tc.ok && got != http.StatusOK {
			t.Errorf("country %s: expected 200, got %d", tc.country, got)
		}
		if !tc.ok && got != http.StatusUnavailableForLegalReasons {
			t.Errorf("country %s: expected 451, got %d", tc.country, got)
		}
	}
}

func TestAllowMode_OverridesBlock(t *testing.T) {
	// When AllowCountries is set, BlockCountries is ignored entirely.
	// Block list includes US; allow list also includes US — US should pass.
	g := newTestMiddleware(t, dynamic.GeoBlock{
		AllowCountries: []string{"US"},
		BlockCountries: []string{"US"},
	}, fakeResolver{country: "US"})

	req := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	req.RemoteAddr = "8.8.8.8:1"
	rw := httptest.NewRecorder()
	g.ServeHTTP(rw, req)

	if rw.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (allow takes precedence), got %d", rw.Result().StatusCode)
	}
}

func TestUpstreamCountryHeader_Honored(t *testing.T) {
	// Trusted upstream proxy (Cloudflare) sets CF-IPCountry. The
	// middleware should honour it and not consult the resolver.
	g := newTestMiddleware(t, dynamic.GeoBlock{
		BlockCountries: []string{"IR"},
	}, fakeResolver{country: "US"}) // resolver would say US

	req := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	req.RemoteAddr = "8.8.8.8:1"
	req.Header.Set("CF-IPCountry", "IR") // but upstream says IR
	rw := httptest.NewRecorder()
	g.ServeHTTP(rw, req)

	if rw.Result().StatusCode != http.StatusUnavailableForLegalReasons {
		t.Fatalf("CF-IPCountry must be honoured; expected 451, got %d", rw.Result().StatusCode)
	}
}

func TestUpstreamCountryHeader_XForwardedCountry(t *testing.T) {
	g := newTestMiddleware(t, dynamic.GeoBlock{}, noopResolver{})

	req := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	req.RemoteAddr = "8.8.8.8:1"
	req.Header.Set("X-Forwarded-Country", "fr")
	rw := httptest.NewRecorder()
	g.ServeHTTP(rw, req)

	if got := rw.Result().Header.Get("Echo-Country"); got != "FR" {
		t.Fatalf("X-Forwarded-Country must be honoured (uppercased); got %q", got)
	}
}

func TestFailOpen_NoResolverNoUpstream(t *testing.T) {
	// No DB, no upstream header → fail-open (request passes, no header set).
	g := newTestMiddleware(t, dynamic.GeoBlock{
		BlockCountries: []string{"IR"},
	}, noopResolver{})

	req := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	req.RemoteAddr = "8.8.8.8:1"
	rw := httptest.NewRecorder()
	g.ServeHTTP(rw, req)

	if rw.Result().StatusCode != http.StatusOK {
		t.Fatalf("fail-open expected 200, got %d", rw.Result().StatusCode)
	}
	if got := rw.Result().Header.Get("Echo-Country"); got != "" {
		t.Fatalf("fail-open should not set country header, got %q", got)
	}
}

func TestCustomRejectStatusCode(t *testing.T) {
	g := newTestMiddleware(t, dynamic.GeoBlock{
		BlockCountries:   []string{"IR"},
		RejectStatusCode: http.StatusForbidden,
	}, fakeResolver{country: "IR"})

	req := httptest.NewRequest(http.MethodGet, "http://x/", nil)
	req.RemoteAddr = "8.8.8.8:1"
	rw := httptest.NewRecorder()
	g.ServeHTTP(rw, req)

	if rw.Result().StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rw.Result().StatusCode)
	}
}

func TestInvalidRejectStatusCode(t *testing.T) {
	_, err := New(context.Background(), nil, dynamic.GeoBlock{
		RejectStatusCode: 999,
	}, "bad")
	if err == nil || !strings.Contains(err.Error(), "invalid HTTP status code") {
		t.Fatalf("expected invalid status code error, got %v", err)
	}
}
