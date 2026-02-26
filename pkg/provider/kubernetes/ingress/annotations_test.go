package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/hanzoai/ingress/v3/pkg/config/dynamic"
	otypes "github.com/hanzoai/ingress/v3/pkg/observability/types"
	"github.com/hanzoai/ingress/v3/pkg/types"
)

func Test_parseRouterConfig(t *testing.T) {
	testCases := []struct {
		desc        string
		annotations map[string]string
		expected    *RouterConfig
	}{
		{
			desc: "router annotations",
			annotations: map[string]string{
				"ingress.kubernetes.io/foo":                                     "bar",
				"ingress.kubernetes.io/foo":                             "bar",
				"ingress.kubernetes.io/router.pathmatcher":              "foobar",
				"ingress.kubernetes.io/router.entrypoints":              "foobar,foobar",
				"ingress.kubernetes.io/router.middlewares":              "foobar,foobar",
				"ingress.kubernetes.io/router.priority":                 "42",
				"ingress.kubernetes.io/router.rulesyntax":               "foobar",
				"ingress.kubernetes.io/router.tls":                      "true",
				"ingress.kubernetes.io/router.tls.certresolver":         "foobar",
				"ingress.kubernetes.io/router.tls.domains.0.main":       "foobar",
				"ingress.kubernetes.io/router.tls.domains.0.sans":       "foobar,foobar",
				"ingress.kubernetes.io/router.tls.domains.1.main":       "foobar",
				"ingress.kubernetes.io/router.tls.domains.1.sans":       "foobar,foobar",
				"ingress.kubernetes.io/router.tls.options":              "foobar",
				"ingress.kubernetes.io/router.observability.accessLogs": "true",
				"ingress.kubernetes.io/router.observability.metrics":    "true",
				"ingress.kubernetes.io/router.observability.tracing":    "true",
			},
			expected: &RouterConfig{
				Router: &RouterIng{
					PathMatcher: "foobar",
					EntryPoints: []string{"foobar", "foobar"},
					Middlewares: []string{"foobar", "foobar"},
					Priority:    42,
					RuleSyntax:  "foobar",
					TLS: &dynamic.RouterTLSConfig{
						CertResolver: "foobar",
						Domains: []types.Domain{
							{
								Main: "foobar",
								SANs: []string{"foobar", "foobar"},
							},
							{
								Main: "foobar",
								SANs: []string{"foobar", "foobar"},
							},
						},
						Options: "foobar",
					},
					Observability: &dynamic.RouterObservabilityConfig{
						AccessLogs:     pointer(true),
						Tracing:        pointer(true),
						Metrics:        pointer(true),
						TraceVerbosity: otypes.MinimalVerbosity,
					},
				},
			},
		},
		{
			desc: "simple TLS annotation",
			annotations: map[string]string{
				"ingress.kubernetes.io/router.tls": "true",
			},
			expected: &RouterConfig{
				Router: &RouterIng{
					PathMatcher: "PathPrefix",
					TLS:         &dynamic.RouterTLSConfig{},
				},
			},
		},
		{
			desc:        "empty map",
			annotations: nil,
			expected:    nil,
		},
		{
			desc:        "nil map",
			annotations: nil,
			expected:    nil,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			cfg, err := parseRouterConfig(test.annotations)
			require.NoError(t, err)

			assert.Equal(t, test.expected, cfg)
		})
	}
}

func Test_parseServiceConfig(t *testing.T) {
	testCases := []struct {
		desc        string
		annotations map[string]string
		expected    *ServiceConfig
	}{
		{
			desc: "service annotations",
			annotations: map[string]string{
				"ingress.kubernetes.io/foo":                                    "bar",
				"ingress.kubernetes.io/foo":                            "bar",
				"ingress.kubernetes.io/service.serversscheme":          "protocol",
				"ingress.kubernetes.io/service.serverstransport":       "foobar@file",
				"ingress.kubernetes.io/service.passhostheader":         "true",
				"ingress.kubernetes.io/service.nativelb":               "true",
				"ingress.kubernetes.io/service.sticky.cookie":          "true",
				"ingress.kubernetes.io/service.sticky.cookie.httponly": "true",
				"ingress.kubernetes.io/service.sticky.cookie.name":     "foobar",
				"ingress.kubernetes.io/service.sticky.cookie.secure":   "true",
				"ingress.kubernetes.io/service.sticky.cookie.samesite": "none",
				"ingress.kubernetes.io/service.sticky.cookie.domain":   "foo.com",
				"ingress.kubernetes.io/service.sticky.cookie.path":     "foobar",
			},
			expected: &ServiceConfig{
				Service: &ServiceIng{
					Sticky: &dynamic.Sticky{
						Cookie: &dynamic.Cookie{
							Name:     "foobar",
							Secure:   true,
							HTTPOnly: true,
							SameSite: "none",
							Domain:   "foo.com",
							Path:     pointer("foobar"),
						},
					},
					ServersScheme:    "protocol",
					ServersTransport: "foobar@file",
					PassHostHeader:   pointer(true),
					NativeLB:         pointer(true),
				},
			},
		},
		{
			desc: "simple sticky annotation",
			annotations: map[string]string{
				"ingress.kubernetes.io/service.sticky.cookie": "true",
			},
			expected: &ServiceConfig{
				Service: &ServiceIng{
					Sticky: &dynamic.Sticky{
						Cookie: &dynamic.Cookie{
							Path: pointer("/"),
						},
					},
					PassHostHeader: pointer(true),
				},
			},
		},
		{
			desc: "service middlewares annotation",
			annotations: map[string]string{
				"ingress.kubernetes.io/service.middlewares": "middleware1,middleware2",
			},
			expected: &ServiceConfig{
				Service: &ServiceIng{
					Middlewares:    []string{"middleware1", "middleware2"},
					PassHostHeader: pointer(true),
				},
			},
		},
		{
			desc:        "empty map",
			annotations: map[string]string{},
			expected:    nil,
		},
		{
			desc:        "nil map",
			annotations: nil,
			expected:    nil,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			cfg, err := parseServiceConfig(test.annotations)
			require.NoError(t, err)

			assert.Equal(t, test.expected, cfg)
		})
	}
}

func Test_convertAnnotations(t *testing.T) {
	testCases := []struct {
		desc        string
		annotations map[string]string
		expected    map[string]string
	}{
		{
			desc: "router annotations",
			annotations: map[string]string{
				"ingress.kubernetes.io/foo":                                     "bar",
				"ingress.kubernetes.io/foo":                             "bar",
				"ingress.kubernetes.io/router.pathmatcher":              "foobar",
				"ingress.kubernetes.io/router.entrypoints":              "foobar,foobar",
				"ingress.kubernetes.io/router.middlewares":              "foobar,foobar",
				"ingress.kubernetes.io/router.priority":                 "42",
				"ingress.kubernetes.io/router.rulesyntax":               "foobar",
				"ingress.kubernetes.io/router.tls":                      "true",
				"ingress.kubernetes.io/router.tls.certresolver":         "foobar",
				"ingress.kubernetes.io/router.tls.domains.0.main":       "foobar",
				"ingress.kubernetes.io/router.tls.domains.0.sans":       "foobar,foobar",
				"ingress.kubernetes.io/router.tls.domains.1.main":       "foobar",
				"ingress.kubernetes.io/router.tls.domains.1.sans":       "foobar,foobar",
				"ingress.kubernetes.io/router.tls.options":              "foobar",
				"ingress.kubernetes.io/router.observability.accessLogs": "true",
				"ingress.kubernetes.io/router.observability.metrics":    "true",
				"ingress.kubernetes.io/router.observability.tracing":    "true",
			},
			expected: map[string]string{
				"ingress.foo":                             "bar",
				"ingress.router.pathmatcher":              "foobar",
				"ingress.router.entrypoints":              "foobar,foobar",
				"ingress.router.middlewares":              "foobar,foobar",
				"ingress.router.priority":                 "42",
				"ingress.router.rulesyntax":               "foobar",
				"ingress.router.tls":                      "true",
				"ingress.router.tls.certresolver":         "foobar",
				"ingress.router.tls.domains[0].main":      "foobar",
				"ingress.router.tls.domains[0].sans":      "foobar,foobar",
				"ingress.router.tls.domains[1].main":      "foobar",
				"ingress.router.tls.domains[1].sans":      "foobar,foobar",
				"ingress.router.tls.options":              "foobar",
				"ingress.router.observability.accessLogs": "true",
				"ingress.router.observability.metrics":    "true",
				"ingress.router.observability.tracing":    "true",
			},
		},
		{
			desc: "service annotations",
			annotations: map[string]string{
				"ingress.kubernetes.io/service.serversscheme":          "protocol",
				"ingress.kubernetes.io/service.serverstransport":       "foobar@file",
				"ingress.kubernetes.io/service.passhostheader":         "true",
				"ingress.kubernetes.io/service.sticky.cookie":          "true",
				"ingress.kubernetes.io/service.sticky.cookie.httponly": "true",
				"ingress.kubernetes.io/service.sticky.cookie.name":     "foobar",
				"ingress.kubernetes.io/service.sticky.cookie.secure":   "true",
			},
			expected: map[string]string{
				"ingress.service.passhostheader":         "true",
				"ingress.service.serversscheme":          "protocol",
				"ingress.service.serverstransport":       "foobar@file",
				"ingress.service.sticky.cookie":          "true",
				"ingress.service.sticky.cookie.httponly": "true",
				"ingress.service.sticky.cookie.name":     "foobar",
				"ingress.service.sticky.cookie.secure":   "true",
			},
		},
		{
			desc:        "empty map",
			annotations: map[string]string{},
			expected:    nil,
		},
		{
			desc:        "nil map",
			annotations: nil,
			expected:    nil,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			labels := convertAnnotations(test.annotations)

			assert.Equal(t, test.expected, labels)
		})
	}
}
