package forwardedheaders

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeHTTP(t *testing.T) {
	testCases := []struct {
		desc              string
		insecure          bool
		trustedIps        []string
		connectionHeaders []string
		incomingHeaders   map[string][]string
		remoteAddr        string
		expectedHeaders   map[string]string
		tls               bool
		websocket         bool
		host              string
	}{
		{
			desc:            "all Empty",
			insecure:        true,
			trustedIps:      nil,
			remoteAddr:      "",
			incomingHeaders: map[string][]string{},
			expectedHeaders: map[string]string{
				xForwardedFor:               "",
				xForwardedURI:               "",
				xForwardedMethod:            "",
				xForwardedTLSClientCert:     "",
				xForwardedTLSClientCertInfo: "",
			},
		},
		{
			desc:       "insecure true with incoming X-Forwarded headers",
			insecure:   true,
			trustedIps: nil,
			remoteAddr: "",
			incomingHeaders: map[string][]string{
				xForwardedFor:               {"10.0.1.0, 10.0.1.12"},
				xForwardedURI:               {"/bar"},
				xForwardedMethod:            {"GET"},
				xForwardedTLSClientCert:     {"Cert"},
				xForwardedTLSClientCertInfo: {"CertInfo"},
				xForwardedPrefix:            {"/prefix"},
			},
			expectedHeaders: map[string]string{
				xForwardedFor:               "10.0.1.0, 10.0.1.12",
				xForwardedURI:               "/bar",
				xForwardedMethod:            "GET",
				xForwardedTLSClientCert:     "Cert",
				xForwardedTLSClientCertInfo: "CertInfo",
				xForwardedPrefix:            "/prefix",
			},
		},
		{
			desc:       "insecure false with incoming X-Forwarded headers",
			insecure:   false,
			trustedIps: nil,
			remoteAddr: "",
			incomingHeaders: map[string][]string{
				xForwardedFor:               {"10.0.1.0, 10.0.1.12"},
				xForwardedURI:               {"/bar"},
				xForwardedMethod:            {"GET"},
				xForwardedTLSClientCert:     {"Cert"},
				xForwardedTLSClientCertInfo: {"CertInfo"},
				xForwardedPrefix:            {"/prefix"},
			},
			expectedHeaders: map[string]string{
				xForwardedFor:               "",
				xForwardedURI:               "",
				xForwardedMethod:            "",
				xForwardedTLSClientCert:     "",
				xForwardedTLSClientCertInfo: "",
				xForwardedPrefix:            "",
			},
		},
		{
			desc:       "insecure false with incoming X-Forwarded headers and valid Trusted Ips",
			insecure:   false,
			trustedIps: []string{"10.0.1.100"},
			remoteAddr: "10.0.1.100:80",
			incomingHeaders: map[string][]string{
				xForwardedFor:               {"10.0.1.0, 10.0.1.12"},
				xForwardedURI:               {"/bar"},
				xForwardedMethod:            {"GET"},
				xForwardedTLSClientCert:     {"Cert"},
				xForwardedTLSClientCertInfo: {"CertInfo"},
				xForwardedPrefix:            {"/prefix"},
			},
			expectedHeaders: map[string]string{
				xForwardedFor:               "10.0.1.0, 10.0.1.12",
				xForwardedURI:               "/bar",
				xForwardedMethod:            "GET",
				xForwardedTLSClientCert:     "Cert",
				xForwardedTLSClientCertInfo: "CertInfo",
				xForwardedPrefix:            "/prefix",
			},
		},
		{
			desc:       "insecure false with incoming X-Forwarded headers and invalid Trusted Ips",
			insecure:   false,
			trustedIps: []string{"10.0.1.100"},
			remoteAddr: "10.0.1.101:80",
			incomingHeaders: map[string][]string{
				xForwardedFor:               {"10.0.1.0, 10.0.1.12"},
				xForwardedURI:               {"/bar"},
				xForwardedMethod:            {"GET"},
				xForwardedTLSClientCert:     {"Cert"},
				xForwardedTLSClientCertInfo: {"CertInfo"},
				xForwardedPrefix:            {"/prefix"},
			},
			expectedHeaders: map[string]string{
				xForwardedFor:               "",
				xForwardedURI:               "",
				xForwardedMethod:            "",
				xForwardedTLSClientCert:     "",
				xForwardedTLSClientCertInfo: "",
				xForwardedPrefix:            "",
			},
		},
		{
			desc:       "insecure false with incoming X-Forwarded headers and valid Trusted Ips CIDR",
			insecure:   false,
			trustedIps: []string{"1.2.3.4/24"},
			remoteAddr: "1.2.3.156:80",
			incomingHeaders: map[string][]string{
				xForwardedFor:               {"10.0.1.0, 10.0.1.12"},
				xForwardedURI:               {"/bar"},
				xForwardedMethod:            {"GET"},
				xForwardedTLSClientCert:     {"Cert"},
				xForwardedTLSClientCertInfo: {"CertInfo"},
				xForwardedPrefix:            {"/prefix"},
			},
			expectedHeaders: map[string]string{
				xForwardedFor:               "10.0.1.0, 10.0.1.12",
				xForwardedURI:               "/bar",
				xForwardedMethod:            "GET",
				xForwardedTLSClientCert:     "Cert",
				xForwardedTLSClientCertInfo: "CertInfo",
				xForwardedPrefix:            "/prefix",
			},
		},
		{
			desc:       "insecure false with incoming X-Forwarded headers and invalid Trusted Ips CIDR",
			insecure:   false,
			trustedIps: []string{"1.2.3.4/24"},
			remoteAddr: "10.0.1.101:80",
			incomingHeaders: map[string][]string{
				xForwardedFor:               {"10.0.1.0, 10.0.1.12"},
				xForwardedURI:               {"/bar"},
				xForwardedMethod:            {"GET"},
				xForwardedTLSClientCert:     {"Cert"},
				xForwardedTLSClientCertInfo: {"CertInfo"},
				xForwardedPrefix:            {"/prefix"},
			},
			expectedHeaders: map[string]string{
				xForwardedFor:               "",
				xForwardedURI:               "",
				xForwardedMethod:            "",
				xForwardedTLSClientCert:     "",
				xForwardedTLSClientCertInfo: "",
				xForwardedPrefix:            "",
			},
		},
		{
			desc:     "xForwardedFor with multiple header(s) values",
			insecure: true,
			incomingHeaders: map[string][]string{
				xForwardedFor: {
					"10.0.0.4, 10.0.0.3",
					"10.0.0.2, 10.0.0.1",
					"10.0.0.0",
				},
			},
			expectedHeaders: map[string]string{
				xForwardedFor: "10.0.0.4, 10.0.0.3, 10.0.0.2, 10.0.0.1, 10.0.0.0",
			},
		},
		{
			desc:       "xRealIP populated from remote address",
			remoteAddr: "10.0.1.101:80",
			expectedHeaders: map[string]string{
				xRealIP: "10.0.1.101",
			},
		},
		{
			desc:       "xRealIP was already populated from previous headers",
			insecure:   true,
			remoteAddr: "10.0.1.101:80",
			incomingHeaders: map[string][]string{
				xRealIP: {"10.0.1.12"},
			},
			expectedHeaders: map[string]string{
				xRealIP: "10.0.1.12",
			},
		},
		{
			desc: "xForwardedProto with no tls",
			tls:  false,
			expectedHeaders: map[string]string{
				xForwardedProto: "http",
			},
		},
		{
			desc: "xForwardedProto with tls",
			tls:  true,
			expectedHeaders: map[string]string{
				xForwardedProto: "https",
			},
		},
		{
			desc:      "xForwardedProto with websocket",
			tls:       false,
			websocket: true,
			expectedHeaders: map[string]string{
				xForwardedProto: "ws",
			},
		},
		{
			desc:      "xForwardedProto with websocket and tls",
			tls:       true,
			websocket: true,
			expectedHeaders: map[string]string{
				xForwardedProto: "wss",
			},
		},
		{
			desc:      "xForwardedProto with websocket and tls and already x-forwarded-proto with wss",
			tls:       true,
			websocket: true,
			incomingHeaders: map[string][]string{
				xForwardedProto: {"wss"},
			},
			expectedHeaders: map[string]string{
				xForwardedProto: "wss",
			},
		},
		{
			desc: "xForwardedPort with explicit port",
			host: "foo.com:8080",
			expectedHeaders: map[string]string{
				xForwardedPort: "8080",
			},
		},
		{
			desc: "xForwardedPort with implicit tls port from proto header",
			// setting insecure just so our initial xForwardedProto does not get cleaned
			insecure: true,
			incomingHeaders: map[string][]string{
				xForwardedProto: {"https"},
			},
			expectedHeaders: map[string]string{
				xForwardedProto: "https",
				xForwardedPort:  "443",
			},
		},
		{
			desc: "xForwardedPort with implicit tls port from TLS in req",
			tls:  true,
			expectedHeaders: map[string]string{
				xForwardedPort: "443",
			},
		},
		{
			desc: "xForwardedHost from req host",
			host: "foo.com:8080",
			expectedHeaders: map[string]string{
				xForwardedHost: "foo.com:8080",
			},
		},
		{
			desc: "xForwardedServer from req XForwarded",
			host: "foo.com:8080",
			expectedHeaders: map[string]string{
				xForwardedServer: "foo.com:8080",
			},
		},
		{
			desc:     "Untrusted: Connection header has no effect on X- forwarded headers",
			insecure: false,
			incomingHeaders: map[string][]string{
				connection: {
					xForwardedProto,
					xForwardedFor,
					xForwardedURI,
					xForwardedMethod,
					xForwardedHost,
					xForwardedPort,
					xForwardedTLSClientCert,
					xForwardedTLSClientCertInfo,
					xForwardedPrefix,
					xRealIP,
				},
				xForwardedProto:             {"foo"},
				xForwardedFor:               {"foo"},
				xForwardedURI:               {"foo"},
				xForwardedMethod:            {"foo"},
				xForwardedHost:              {"foo"},
				xForwardedPort:              {"foo"},
				xForwardedTLSClientCert:     {"foo"},
				xForwardedTLSClientCertInfo: {"foo"},
				xForwardedPrefix:            {"foo"},
				xRealIP:                     {"foo"},
			},
			expectedHeaders: map[string]string{
				xForwardedProto:             "http",
				xForwardedFor:               "",
				xForwardedURI:               "",
				xForwardedMethod:            "",
				xForwardedHost:              "",
				xForwardedPort:              "80",
				xForwardedTLSClientCert:     "",
				xForwardedTLSClientCertInfo: "",
				xForwardedPrefix:            "",
				xRealIP:                     "",
				connection:                  "",
			},
		},
		{
			desc:     "Trusted (insecure): Connection header has no effect on X- forwarded headers",
			insecure: true,
			incomingHeaders: map[string][]string{
				connection: {
					xForwardedProto,
					xForwardedFor,
					xForwardedURI,
					xForwardedMethod,
					xForwardedHost,
					xForwardedPort,
					xForwardedTLSClientCert,
					xForwardedTLSClientCertInfo,
					xForwardedPrefix,
					xRealIP,
				},
				xForwardedProto:             {"foo"},
				xForwardedFor:               {"foo"},
				xForwardedURI:               {"foo"},
				xForwardedMethod:            {"foo"},
				xForwardedHost:              {"foo"},
				xForwardedPort:              {"foo"},
				xForwardedTLSClientCert:     {"foo"},
				xForwardedTLSClientCertInfo: {"foo"},
				xForwardedPrefix:            {"foo"},
				xRealIP:                     {"foo"},
			},
			expectedHeaders: map[string]string{
				xForwardedProto:             "foo",
				xForwardedFor:               "foo",
				xForwardedURI:               "foo",
				xForwardedMethod:            "foo",
				xForwardedHost:              "foo",
				xForwardedPort:              "foo",
				xForwardedTLSClientCert:     "foo",
				xForwardedTLSClientCertInfo: "foo",
				xForwardedPrefix:            "foo",
				xRealIP:                     "foo",
				connection:                  "",
			},
		},
		{
			desc:     "Untrusted and Connection: Connection header has no effect on X- forwarded headers",
			insecure: false,
			connectionHeaders: []string{
				xForwardedProto,
				xForwardedFor,
				xForwardedURI,
				xForwardedMethod,
				xForwardedHost,
				xForwardedPort,
				xForwardedTLSClientCert,
				xForwardedTLSClientCertInfo,
				xForwardedPrefix,
				xRealIP,
			},
			incomingHeaders: map[string][]string{
				connection: {
					xForwardedProto,
					xForwardedFor,
					xForwardedURI,
					xForwardedMethod,
					xForwardedHost,
					xForwardedPort,
					xForwardedTLSClientCert,
					xForwardedTLSClientCertInfo,
					xForwardedPrefix,
					xRealIP,
				},
				xForwardedProto:             {"foo"},
				xForwardedFor:               {"foo"},
				xForwardedURI:               {"foo"},
				xForwardedMethod:            {"foo"},
				xForwardedHost:              {"foo"},
				xForwardedPort:              {"foo"},
				xForwardedTLSClientCert:     {"foo"},
				xForwardedTLSClientCertInfo: {"foo"},
				xForwardedPrefix:            {"foo"},
				xRealIP:                     {"foo"},
			},
			expectedHeaders: map[string]string{
				xForwardedProto:             "http",
				xForwardedFor:               "",
				xForwardedURI:               "",
				xForwardedMethod:            "",
				xForwardedHost:              "",
				xForwardedPort:              "80",
				xForwardedTLSClientCert:     "",
				xForwardedTLSClientCertInfo: "",
				xForwardedPrefix:            "",
				xRealIP:                     "",
				connection:                  "",
			},
		},
		{
			desc:     "Trusted (insecure) and Connection: Connection header has no effect on X- forwarded headers",
			insecure: true,
			connectionHeaders: []string{
				xForwardedProto,
				xForwardedFor,
				xForwardedURI,
				xForwardedMethod,
				xForwardedHost,
				xForwardedPort,
				xForwardedTLSClientCert,
				xForwardedTLSClientCertInfo,
				xForwardedPrefix,
				xRealIP,
			},
			incomingHeaders: map[string][]string{
				connection: {
					xForwardedProto,
					xForwardedFor,
					xForwardedURI,
					xForwardedMethod,
					xForwardedHost,
					xForwardedPort,
					xForwardedTLSClientCert,
					xForwardedTLSClientCertInfo,
					xForwardedPrefix,
					xRealIP,
				},
				xForwardedProto:             {"foo"},
				xForwardedFor:               {"foo"},
				xForwardedURI:               {"foo"},
				xForwardedMethod:            {"foo"},
				xForwardedHost:              {"foo"},
				xForwardedPort:              {"foo"},
				xForwardedTLSClientCert:     {"foo"},
				xForwardedTLSClientCertInfo: {"foo"},
				xForwardedPrefix:            {"foo"},
				xRealIP:                     {"foo"},
			},
			expectedHeaders: map[string]string{
				xForwardedProto:             "foo",
				xForwardedFor:               "foo",
				xForwardedURI:               "foo",
				xForwardedMethod:            "foo",
				xForwardedHost:              "foo",
				xForwardedPort:              "foo",
				xForwardedTLSClientCert:     "foo",
				xForwardedTLSClientCertInfo: "foo",
				xForwardedPrefix:            "foo",
				xRealIP:                     "foo",
				connection:                  "",
			},
		},
		{
			desc:     "Trusted (insecure) and Connection: Testing case sensitivity on connection Headers param",
			insecure: true,
			connectionHeaders: []string{
				strings.ToLower(xForwardedProto),
				strings.ToLower(xForwardedFor),
				strings.ToLower(xForwardedURI),
				strings.ToLower(xForwardedMethod),
				strings.ToLower(xForwardedHost),
				strings.ToLower(xForwardedPort),
				strings.ToLower(xForwardedTLSClientCert),
				strings.ToLower(xForwardedTLSClientCertInfo),
				strings.ToLower(xForwardedPrefix),
				strings.ToLower(xRealIP),
			},
			incomingHeaders: map[string][]string{
				connection: {
					xForwardedProto,
					xForwardedFor,
					xForwardedURI,
					xForwardedMethod,
					xForwardedHost,
					xForwardedPort,
					xForwardedTLSClientCert,
					xForwardedTLSClientCertInfo,
					xForwardedPrefix,
					xRealIP,
				},
				xForwardedProto:             {"foo"},
				xForwardedFor:               {"foo"},
				xForwardedURI:               {"foo"},
				xForwardedMethod:            {"foo"},
				xForwardedHost:              {"foo"},
				xForwardedPort:              {"foo"},
				xForwardedTLSClientCert:     {"foo"},
				xForwardedTLSClientCertInfo: {"foo"},
				xForwardedPrefix:            {"foo"},
				xRealIP:                     {"foo"},
			},
			expectedHeaders: map[string]string{
				xForwardedProto:             "foo",
				xForwardedFor:               "foo",
				xForwardedURI:               "foo",
				xForwardedMethod:            "foo",
				xForwardedHost:              "foo",
				xForwardedPort:              "foo",
				xForwardedTLSClientCert:     "foo",
				xForwardedTLSClientCertInfo: "foo",
				xForwardedPrefix:            "foo",
				xRealIP:                     "foo",
				connection:                  "",
			},
		},
		{
			desc:     "Trusted (insecure) and Connection: Testing case sensitivity on X- forwarded headers",
			insecure: true,
			incomingHeaders: map[string][]string{
				connection: {
					strings.ToLower(xForwardedProto),
					strings.ToLower(xForwardedFor),
					strings.ToLower(xForwardedURI),
					strings.ToLower(xForwardedMethod),
					strings.ToLower(xForwardedHost),
					strings.ToLower(xForwardedPort),
					strings.ToLower(xForwardedTLSClientCert),
					strings.ToLower(xForwardedTLSClientCertInfo),
					strings.ToLower(xForwardedPrefix),
					strings.ToLower(xRealIP),
				},
				xForwardedProto:             {"foo"},
				xForwardedFor:               {"foo"},
				xForwardedURI:               {"foo"},
				xForwardedMethod:            {"foo"},
				xForwardedHost:              {"foo"},
				xForwardedPort:              {"foo"},
				xForwardedTLSClientCert:     {"foo"},
				xForwardedTLSClientCertInfo: {"foo"},
				xForwardedPrefix:            {"foo"},
				xRealIP:                     {"foo"},
			},
			expectedHeaders: map[string]string{
				xForwardedProto:             "foo",
				xForwardedFor:               "foo",
				xForwardedURI:               "foo",
				xForwardedMethod:            "foo",
				xForwardedHost:              "foo",
				xForwardedPort:              "foo",
				xForwardedTLSClientCert:     "foo",
				xForwardedTLSClientCertInfo: "foo",
				xForwardedPrefix:            "foo",
				xRealIP:                     "foo",
				connection:                  "",
			},
		},
		{
			desc: "Connection: one remove, and one passthrough header",
			connectionHeaders: []string{
				"foo",
			},
			incomingHeaders: map[string][]string{
				connection: {
					"foo",
					"bar",
				},
				"Foo": {"bar"},
				"Bar": {"foo"},
			},
			expectedHeaders: map[string]string{
				"Bar": "",
				"Foo": "bar",
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequest(http.MethodGet, "", nil)
			require.NoError(t, err)

			req.RemoteAddr = test.remoteAddr

			if test.tls {
				req.TLS = &tls.ConnectionState{}
			}

			if test.websocket {
				req.Header.Set(connection, "upgrade")
				req.Header.Set(upgrade, "websocket")
			}

			if test.host != "" {
				req.Host = test.host
			}

			for k, values := range test.incomingHeaders {
				for _, value := range values {
					req.Header.Add(k, value)
				}
			}

			m, err := NewXForwarded(test.insecure, test.trustedIps, test.connectionHeaders, false,
				http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
			require.NoError(t, err)

			if test.host != "" {
				m.hostname = test.host
			}

			m.ServeHTTP(nil, req)

			for k, v := range test.expectedHeaders {
				assert.Equal(t, v, req.Header.Get(k))
			}
		})
	}
}

func Test_isWebsocketRequest(t *testing.T) {
	testCases := []struct {
		desc             string
		connectionHeader string
		upgradeHeader    string
		assert           assert.BoolAssertionFunc
	}{
		{
			desc:             "connection Header multiple values middle",
			connectionHeader: "foo,upgrade,bar",
			upgradeHeader:    "websocket",
			assert:           assert.True,
		},
		{
			desc:             "connection Header multiple values end",
			connectionHeader: "foo,bar,upgrade",
			upgradeHeader:    "websocket",
			assert:           assert.True,
		},
		{
			desc:             "connection Header multiple values begin",
			connectionHeader: "upgrade,foo,bar",
			upgradeHeader:    "websocket",
			assert:           assert.True,
		},
		{
			desc:             "connection Header no upgrade",
			connectionHeader: "foo,bar",
			upgradeHeader:    "websocket",
			assert:           assert.False,
		},
		{
			desc:             "connection Header empty",
			connectionHeader: "",
			upgradeHeader:    "websocket",
			assert:           assert.False,
		},
		{
			desc:             "no header values",
			connectionHeader: "foo,bar",
			upgradeHeader:    "foo,bar",
			assert:           assert.False,
		},
		{
			desc:             "upgrade header multiple values",
			connectionHeader: "upgrade",
			upgradeHeader:    "foo,bar,websocket",
			assert:           assert.True,
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)

			req.Header.Set(connection, test.connectionHeader)
			req.Header.Set(upgrade, test.upgradeHeader)

			ok := isWebsocketRequest(req)

			test.assert(t, ok)
		})
	}
}

func TestConnection(t *testing.T) {
	testCases := []struct {
		desc              string
		reqHeaders        map[string]string
		connectionHeaders []string
		expected          http.Header
	}{
		{
			desc: "simple remove",
			reqHeaders: map[string]string{
				"Foo":      "bar",
				connection: "foo",
			},
			expected: http.Header{},
		},
		{
			desc: "remove and upgrade",
			reqHeaders: map[string]string{
				upgrade:    "test",
				"Foo":      "bar",
				connection: "upgrade,foo",
			},
			expected: http.Header{
				upgrade:    []string{"test"},
				connection: []string{"Upgrade"},
			},
		},
		{
			desc: "no remove",
			reqHeaders: map[string]string{
				"Foo":      "bar",
				connection: "fii",
			},
			expected: http.Header{
				"Foo": []string{"bar"},
			},
		},
		{
			desc: "no remove because connection header pass through",
			reqHeaders: map[string]string{
				"Foo":      "bar",
				connection: "Foo",
			},
			connectionHeaders: []string{"Foo"},
			expected: http.Header{
				"Foo":      []string{"bar"},
				connection: []string{"Foo"},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			forwarded, err := NewXForwarded(true, nil, test.connectionHeaders, false, nil)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodGet, "https://localhost", nil)

			for k, v := range test.reqHeaders {
				req.Header.Set(k, v)
			}

			forwarded.removeConnectionHeaders(req)

			assert.Equal(t, test.expected, req.Header)
		})
	}
}

// A proxy's claim about the client survives only from a trusted peer. From any
// other peer — a host on the LAN reaching the entrypoint around the CDN — the
// claim is the client describing itself, and it must not reach the backend.
func TestProxyClaims(t *testing.T) {
	cloudflare := []string{"173.245.48.0/20"}

	claims := map[string]string{
		"CF-Connecting-IP":         "203.0.113.7",
		"CF-Connecting-IPv6":       "2001:db8::7",
		"CF-IPCountry":             "NZ",
		"CF-IPCity":                "Auckland",
		"CF-Visitor":               `{"scheme":"https"}`,
		"cf_connecting_ip":         "203.0.113.8",
		"True-Client-IP":           "192.0.2.5",
		"X-Real-IP":                "192.0.2.6",
		"X-Client-IP":              "192.0.2.7",
		"X-Cluster-Client-IP":      "192.0.2.8",
		"Fastly-Client-IP":         "192.0.2.9",
		"Forwarded":                "for=192.0.2.10",
		"Forwarded-For":            "192.0.2.11",
		"X-Forwarded":              "for=192.0.2.12",
		"X-Original-Forwarded-For": "192.0.2.13",
		"X-Forwarded-For":          "198.51.100.9",
		"X_Forwarded_For":          "198.51.100.10",
		"X-Forwarded-Country":      "NZ",
		"X-Forwarded-User":         "admin",
		"X-AppEngine-Country":      "NZ",
	}

	testCases := []struct {
		desc       string
		insecure   bool
		remoteAddr string
		kept       bool
	}{
		{desc: "LAN host", remoteAddr: "10.0.0.132:51000"},
		{desc: "LAN public address", remoteAddr: "174.160.143.53:51000"},
		{desc: "loopback", remoteAddr: "127.0.0.1:51000"},
		{desc: "IPv4-mapped LAN host", remoteAddr: "[::ffff:10.0.0.132]:51000"},
		{desc: "unparseable peer", remoteAddr: "not-an-address"},
		{desc: "Cloudflare", remoteAddr: "173.245.48.10:51000", kept: true},
		{desc: "insecure", insecure: true, remoteAddr: "10.0.0.132:51000", kept: true},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "https://api.hanzo.ai/v1/x", nil)
			req.RemoteAddr = test.remoteAddr
			for k, v := range claims {
				req.Header.Add(k, v)
			}
			req.Header.Set("Authorization", "Bearer keep")
			req.Header.Set("Cfg-Version", "keep")

			m, err := NewXForwarded(test.insecure, cloudflare, nil, false,
				http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
			require.NoError(t, err)
			m.ServeHTTP(nil, req)

			for k, v := range claims {
				got := req.Header.Get(k)
				switch {
				case test.kept:
					assert.Equal(t, v, got, k)
				case http.CanonicalHeaderKey(k) == xRealIP:
					// Rewritten to the peer, never the client's value.
					assert.NotEqual(t, v, got, k)
				default:
					assert.Empty(t, got, k)
				}
			}
			// Not claims: an unrelated header, and one that only shares a prefix.
			assert.Equal(t, "Bearer keep", req.Header.Get("Authorization"))
			assert.Equal(t, "keep", req.Header.Get("Cfg-Version"))
		})
	}
}

// What the backend receives through a real listener and reverse proxy: a LAN
// client's claims are gone and X-Forwarded-For is the socket peer alone.
func TestProxyClaimsOnTheWire(t *testing.T) {
	var got http.Header
	backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	t.Cleanup(backend.Close)

	target, err := url.Parse(backend.URL)
	require.NoError(t, err)
	m, err := NewXForwarded(false, []string{"173.245.48.0/20"}, nil, false, httputil.NewSingleHostReverseProxy(target))
	require.NoError(t, err)
	edge := httptest.NewServer(m)
	t.Cleanup(edge.Close)

	req, err := http.NewRequest(http.MethodGet, edge.URL+"/v1/x", nil)
	require.NoError(t, err)
	req.Header.Set("CF-Connecting-IP", "203.0.113.7")
	req.Header.Set("CF-IPCountry", "NZ")
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	req.Header.Set("True-Client-IP", "192.0.2.5")
	req.Header.Set("Forwarded", "for=192.0.2.8")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())

	assert.Equal(t, "127.0.0.1", got.Get("X-Forwarded-For"))
	assert.Equal(t, "127.0.0.1", got.Get("X-Real-Ip"))
	for _, h := range []string{"CF-Connecting-IP", "CF-IPCountry", "True-Client-IP", "Forwarded"} {
		assert.Empty(t, got.Get(h), h)
	}
}
