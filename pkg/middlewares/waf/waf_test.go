package waf

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
)

func serve(t *testing.T, config dynamic.WAF, target string) (int, bool) {
	t.Helper()

	reached := false
	next := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		reached = true
		rw.WriteHeader(http.StatusOK)
	})

	h, err := New(context.Background(), next, config, "waf")
	require.NoError(t, err)

	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, target, nil))
	return rw.Code, reached
}

// A ruleset that loads nothing inspects nothing while answering every request
// successfully, so it is refused where it is built rather than at the edge.
func TestNoRulesIsRefused(t *testing.T) {
	_, err := New(context.Background(), http.NotFoundHandler(), dynamic.WAF{}, "waf")
	assert.Error(t, err)

	// The control: the same call with one rule builds.
	_, err = New(context.Background(), http.NotFoundHandler(),
		dynamic.WAF{Directives: `SecRule REQUEST_URI "@contains /nope" "id:1,phase:1,deny,status:403"`}, "waf")
	assert.NoError(t, err)
}

func TestInlineRuleRefusesAndTheNextHandlerNeverRuns(t *testing.T) {
	config := dynamic.WAF{
		Directives: `SecRule REQUEST_URI "@contains /admin" "id:1,phase:1,deny,status:403"`,
	}

	code, reached := serve(t, config, "http://x/admin")
	assert.Equal(t, http.StatusForbidden, code)
	assert.False(t, reached, "a refused request must not reach the service")

	code, reached = serve(t, config, "http://x/ok")
	assert.Equal(t, http.StatusOK, code)
	assert.True(t, reached)
}

// DetectionOnly is how a ruleset is measured against live traffic: the same
// rule matches and the request still proceeds.
func TestDetectionOnlyRefusesNothing(t *testing.T) {
	config := dynamic.WAF{
		Directives:    `SecRule REQUEST_URI "@contains /admin" "id:1,phase:1,deny,status:403"`,
		DetectionOnly: true,
	}

	code, reached := serve(t, config, "http://x/admin")
	assert.Equal(t, http.StatusOK, code)
	assert.True(t, reached)
}

// The embedded Core Rule Set is loaded with the setup file it reads, so the
// anomaly thresholds the rules score against are actually set.
func TestCoreRuleSetRefusesInjection(t *testing.T) {
	config := dynamic.WAF{CoreRuleSet: true}

	code, reached := serve(t, config, "http://x/?id=1%27%20OR%20%271%27%3D%271")
	assert.Equal(t, http.StatusForbidden, code, "SQL injection must be refused")
	assert.False(t, reached)

	code, reached = serve(t, config, "http://x/?id=7")
	assert.Equal(t, http.StatusOK, code, "an ordinary request must pass")
	assert.True(t, reached)
}

func TestCoreRuleSetRefusesTraversal(t *testing.T) {
	code, reached := serve(t, dynamic.WAF{CoreRuleSet: true}, "http://x/?f=../../../../etc/passwd")
	assert.Equal(t, http.StatusForbidden, code)
	assert.False(t, reached)
}

// The engine state is this middleware's to decide. A ruleset that names its
// own — as the Core Rule Set's recommended configuration does on line 7 —
// must not be able to disarm the firewall that loaded it.
func TestARulesetCannotDisarmTheFirewall(t *testing.T) {
	config := dynamic.WAF{
		Directives: `SecRuleEngine DetectionOnly
SecRule REQUEST_URI "@contains /admin" "id:1,phase:1,deny,status:403"`,
	}

	code, reached := serve(t, config, "http://x/admin")
	assert.Equal(t, http.StatusForbidden, code)
	assert.False(t, reached)
}
