package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/hanzoai/ingress/pkg/config/runtime"
	"github.com/hanzoai/ingress/pkg/config/static"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoAPIPrefix pins the admin API under /v1/ingress/. No route the handler
// registers begins with /api, and the retired /api/ spellings answer 404 while
// their /v1/ingress/ counterparts answer 200.
func TestNoAPIPrefix(t *testing.T) {
	cfg := static.Configuration{API: &static.API{Debug: true}, Global: &static.Global{}}
	router := New(cfg, &runtime.Configuration{}).createRouter()

	err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tpl, err := route.GetPathTemplate()
		if err != nil {
			return nil
		}
		if strings.HasPrefix(tpl, "/api") {
			t.Errorf("route %q is registered under /api; the admin API lives under /v1/ingress/", tpl)
		}
		return nil
	})
	require.NoError(t, err)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	for _, p := range []string{"rawdata", "overview", "version", "entrypoints", "http/routers", "tcp/routers", "udp/routers"} {
		code := get(t, server.URL+"/api/"+p)
		assert.Equal(t, http.StatusNotFound, code, "/api/%s", p)

		code = get(t, server.URL+"/v1/ingress/"+p)
		assert.Equal(t, http.StatusOK, code, "/v1/ingress/%s", p)
	}
}

func get(t *testing.T, url string) int {
	t.Helper()

	resp, err := http.Get(url)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	return resp.StatusCode
}
