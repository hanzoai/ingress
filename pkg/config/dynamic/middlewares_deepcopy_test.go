package dynamic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A field the generated deep copy does not carry is dropped in silence on the
// Kubernetes path, so the middleware reads as configured and does nothing.
// geoBlock was in that state: declared, buildable, and absent from every
// Middleware the CRD provider produced.
func TestEveryMiddlewareFieldSurvivesADeepCopy(t *testing.T) {
	in := &Middleware{
		GeoBlock: &GeoBlock{
			BlockCountries:   []string{"KP", "IR"},
			HeaderName:       "X-Geo-Country",
			RejectStatusCode: 451,
		},
		WAF: &WAF{
			CoreRuleSet:     true,
			DirectivesFiles: []string{"/etc/waf/local.conf"},
			Directives:      `SecRule REQUEST_URI "@contains /admin" "id:1,phase:1,deny"`,
		},
	}

	out := in.DeepCopy()

	require.NotNil(t, out.GeoBlock, "geoBlock must survive the copy")
	assert.Equal(t, []string{"KP", "IR"}, out.GeoBlock.BlockCountries)
	assert.Equal(t, 451, out.GeoBlock.RejectStatusCode)

	require.NotNil(t, out.WAF, "waf must survive the copy")
	assert.True(t, out.WAF.CoreRuleSet)
	assert.Equal(t, []string{"/etc/waf/local.conf"}, out.WAF.DirectivesFiles)
	assert.Equal(t, in.WAF.Directives, out.WAF.Directives)

	// The slices are copies, not shared backing arrays.
	out.GeoBlock.BlockCountries[0] = "US"
	out.WAF.DirectivesFiles[0] = "/other"
	assert.Equal(t, "KP", in.GeoBlock.BlockCountries[0])
	assert.Equal(t, "/etc/waf/local.conf", in.WAF.DirectivesFiles[0])
}
