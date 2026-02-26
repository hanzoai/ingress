package label

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ptypes "github.com/traefik/paerser/types"
	"github.com/hanzoai/ingress/v3/pkg/config/dynamic"
	"github.com/hanzoai/ingress/v3/pkg/tls"
	"github.com/hanzoai/ingress/v3/pkg/types"
)

func pointer[T any](v T) *T { return &v }

func TestDecodeConfiguration(t *testing.T) {
	labels := map[string]string{
		"ingress.http.middlewares.Middleware0.addprefix.prefix":                                    "foobar",
		"ingress.http.middlewares.Middleware1.basicauth.headerfield":                               "foobar",
		"ingress.http.middlewares.Middleware1.basicauth.realm":                                     "foobar",
		"ingress.http.middlewares.Middleware1.basicauth.removeheader":                              "true",
		"ingress.http.middlewares.Middleware1.basicauth.users":                                     "foobar, fiibar",
		"ingress.http.middlewares.Middleware1.basicauth.usersfile":                                 "foobar",
		"ingress.http.middlewares.Middleware2.buffering.maxrequestbodybytes":                       "42",
		"ingress.http.middlewares.Middleware2.buffering.maxresponsebodybytes":                      "42",
		"ingress.http.middlewares.Middleware2.buffering.memrequestbodybytes":                       "42",
		"ingress.http.middlewares.Middleware2.buffering.memresponsebodybytes":                      "42",
		"ingress.http.middlewares.Middleware2.buffering.retryexpression":                           "foobar",
		"ingress.http.middlewares.Middleware3.chain.middlewares":                                   "foobar, fiibar",
		"ingress.http.middlewares.Middleware4.circuitbreaker.expression":                           "foobar",
		"ingress.HTTP.Middlewares.Middleware4.circuitbreaker.checkperiod":                          "1s",
		"ingress.HTTP.Middlewares.Middleware4.circuitbreaker.fallbackduration":                     "1s",
		"ingress.HTTP.Middlewares.Middleware4.circuitbreaker.recoveryduration":                     "1s",
		"ingress.HTTP.Middlewares.Middleware4.circuitbreaker.responsecode":                         "403",
		"ingress.http.middlewares.Middleware5.digestauth.headerfield":                              "foobar",
		"ingress.http.middlewares.Middleware5.digestauth.realm":                                    "foobar",
		"ingress.http.middlewares.Middleware5.digestauth.removeheader":                             "true",
		"ingress.http.middlewares.Middleware5.digestauth.users":                                    "foobar, fiibar",
		"ingress.http.middlewares.Middleware5.digestauth.usersfile":                                "foobar",
		"ingress.http.middlewares.Middleware6.errors.query":                                        "foobar",
		"ingress.http.middlewares.Middleware6.errors.service":                                      "foobar",
		"ingress.http.middlewares.Middleware6.errors.status":                                       "foobar, fiibar",
		"ingress.http.middlewares.Middleware7.forwardauth.address":                                 "foobar",
		"ingress.http.middlewares.Middleware7.forwardauth.authresponseheaders":                     "foobar, fiibar",
		"ingress.http.middlewares.Middleware7.forwardauth.authrequestheaders":                      "foobar, fiibar",
		"ingress.http.middlewares.Middleware7.forwardauth.tls.ca":                                  "foobar",
		"ingress.http.middlewares.Middleware7.forwardauth.tls.caoptional":                          "true",
		"ingress.http.middlewares.Middleware7.forwardauth.tls.cert":                                "foobar",
		"ingress.http.middlewares.Middleware7.forwardauth.tls.insecureskipverify":                  "true",
		"ingress.http.middlewares.Middleware7.forwardauth.tls.key":                                 "foobar",
		"ingress.http.middlewares.Middleware7.forwardauth.trustforwardheader":                      "true",
		"ingress.http.middlewares.Middleware7.forwardauth.forwardbody":                             "true",
		"ingress.http.middlewares.Middleware7.forwardauth.maxbodysize":                             "42",
		"ingress.http.middlewares.Middleware7.forwardauth.preserveRequestMethod":                   "true",
		"ingress.http.middlewares.Middleware7.forwardauth.maxresponsebodysize":                     "42",
		"ingress.http.middlewares.Middleware8.headers.accesscontrolallowcredentials":               "true",
		"ingress.http.middlewares.Middleware8.headers.allowedhosts":                                "foobar, fiibar",
		"ingress.http.middlewares.Middleware8.headers.accesscontrolallowheaders":                   "X-foobar, X-fiibar",
		"ingress.http.middlewares.Middleware8.headers.accesscontrolallowmethods":                   "GET, PUT",
		"ingress.http.middlewares.Middleware8.headers.accesscontrolalloworiginList":                "foobar, fiibar",
		"ingress.http.middlewares.Middleware8.headers.accesscontrolalloworiginListRegex":           "foobar, fiibar",
		"ingress.http.middlewares.Middleware8.headers.accesscontrolexposeheaders":                  "X-foobar, X-fiibar",
		"ingress.http.middlewares.Middleware8.headers.accesscontrolmaxage":                         "200",
		"ingress.http.middlewares.Middleware8.headers.addvaryheader":                               "true",
		"ingress.http.middlewares.Middleware8.headers.browserxssfilter":                            "true",
		"ingress.http.middlewares.Middleware8.headers.contentsecuritypolicy":                       "foobar",
		"ingress.http.middlewares.Middleware8.headers.contentsecuritypolicyreportonly":             "foobar",
		"ingress.http.middlewares.Middleware8.headers.contenttypenosniff":                          "true",
		"ingress.http.middlewares.Middleware8.headers.custombrowserxssvalue":                       "foobar",
		"ingress.http.middlewares.Middleware8.headers.customframeoptionsvalue":                     "foobar",
		"ingress.http.middlewares.Middleware8.headers.customrequestheaders.name0":                  "foobar",
		"ingress.http.middlewares.Middleware8.headers.customrequestheaders.name1":                  "foobar",
		"ingress.http.middlewares.Middleware8.headers.customresponseheaders.name0":                 "foobar",
		"ingress.http.middlewares.Middleware8.headers.customresponseheaders.name1":                 "foobar",
		"ingress.http.middlewares.Middleware8.headers.forcestsheader":                              "true",
		"ingress.http.middlewares.Middleware8.headers.framedeny":                                   "true",
		"ingress.http.middlewares.Middleware8.headers.hostsproxyheaders":                           "foobar, fiibar",
		"ingress.http.middlewares.Middleware8.headers.isdevelopment":                               "true",
		"ingress.http.middlewares.Middleware8.headers.publickey":                                   "foobar",
		"ingress.http.middlewares.Middleware8.headers.referrerpolicy":                              "foobar",
		"ingress.http.middlewares.Middleware8.headers.featurepolicy":                               "foobar",
		"ingress.http.middlewares.Middleware8.headers.permissionspolicy":                           "foobar",
		"ingress.http.middlewares.Middleware8.headers.sslforcehost":                                "true",
		"ingress.http.middlewares.Middleware8.headers.sslhost":                                     "foobar",
		"ingress.http.middlewares.Middleware8.headers.sslproxyheaders.name0":                       "foobar",
		"ingress.http.middlewares.Middleware8.headers.sslproxyheaders.name1":                       "foobar",
		"ingress.http.middlewares.Middleware8.headers.sslredirect":                                 "true",
		"ingress.http.middlewares.Middleware8.headers.ssltemporaryredirect":                        "true",
		"ingress.http.middlewares.Middleware8.headers.stsincludesubdomains":                        "true",
		"ingress.http.middlewares.Middleware8.headers.stspreload":                                  "true",
		"ingress.http.middlewares.Middleware8.headers.stsseconds":                                  "42",
		"ingress.http.middlewares.Middleware9.ipallowlist.ipstrategy.depth":                        "42",
		"ingress.http.middlewares.Middleware9.ipallowlist.ipstrategy.excludedips":                  "foobar, fiibar",
		"ingress.http.middlewares.Middleware9.ipallowlist.ipstrategy.ipv6subnet":                   "42",
		"ingress.http.middlewares.Middleware9.ipallowlist.sourcerange":                             "foobar, fiibar",
		"ingress.http.middlewares.Middleware10.inflightreq.amount":                                 "42",
		"ingress.http.middlewares.Middleware10.inflightreq.sourcecriterion.ipstrategy.depth":       "42",
		"ingress.http.middlewares.Middleware10.inflightreq.sourcecriterion.ipstrategy.excludedips": "foobar, fiibar",
		"ingress.http.middlewares.Middleware10.inflightreq.sourcecriterion.ipstrategy.ipv6subnet":  "42",
		"ingress.http.middlewares.Middleware10.inflightreq.sourcecriterion.requestheadername":      "foobar",
		"ingress.http.middlewares.Middleware10.inflightreq.sourcecriterion.requesthost":            "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.notafter":                    "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.notbefore":                   "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.sans":                        "true",
		"ingress.http.middlewares.Middleware11.passTLSClientCert.info.serialNumber":                "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.subject.commonname":          "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.subject.country":             "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.subject.domaincomponent":     "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.subject.locality":            "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.subject.organization":        "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.subject.organizationalunit":  "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.subject.province":            "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.subject.serialnumber":        "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.issuer.commonname":           "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.issuer.country":              "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.issuer.domaincomponent":      "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.issuer.locality":             "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.issuer.organization":         "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.issuer.province":             "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.info.issuer.serialnumber":         "true",
		"ingress.http.middlewares.Middleware11.passtlsclientcert.pem":                              "true",
		"ingress.http.middlewares.Middleware12.ratelimit.average":                                  "42",
		"ingress.http.middlewares.Middleware12.ratelimit.period":                                   "1s",
		"ingress.http.middlewares.Middleware12.ratelimit.burst":                                    "42",
		"ingress.http.middlewares.Middleware12.ratelimit.sourcecriterion.requestheadername":        "foobar",
		"ingress.http.middlewares.Middleware12.ratelimit.sourcecriterion.requesthost":              "true",
		"ingress.http.middlewares.Middleware12.ratelimit.sourcecriterion.ipstrategy.depth":         "42",
		"ingress.http.middlewares.Middleware12.ratelimit.sourcecriterion.ipstrategy.excludedips":   "foobar, foobar",
		"ingress.http.middlewares.Middleware12.ratelimit.sourcecriterion.ipstrategy.ipv6subnet":    "42",
		"ingress.http.middlewares.Middleware13.redirectregex.permanent":                            "true",
		"ingress.http.middlewares.Middleware13.redirectregex.regex":                                "foobar",
		"ingress.http.middlewares.Middleware13.redirectregex.replacement":                          "foobar",
		"ingress.http.middlewares.Middleware13b.redirectscheme.scheme":                             "https",
		"ingress.http.middlewares.Middleware13b.redirectscheme.port":                               "80",
		"ingress.http.middlewares.Middleware13b.redirectscheme.permanent":                          "true",
		"ingress.http.middlewares.Middleware14.replacepath.path":                                   "foobar",
		"ingress.http.middlewares.Middleware15.replacepathregex.regex":                             "foobar",
		"ingress.http.middlewares.Middleware15.replacepathregex.replacement":                       "foobar",
		"ingress.http.middlewares.Middleware16.retry.attempts":                                     "42",
		"ingress.http.middlewares.Middleware16.retry.initialinterval":                              "1s",
		"ingress.http.middlewares.Middleware16.retry.timeout":                                      "1s",
		"ingress.http.middlewares.Middleware16.retry.maxRequestBodyBytes":                          "42",
		"ingress.http.middlewares.Middleware16.retry.status":                                       "foobar, foobar",
		"ingress.http.middlewares.Middleware16.retry.disableRetryOnNetworkError":                   "true",
		"ingress.http.middlewares.Middleware16.retry.retryNonIdempotentMethod":                     "true",
		"ingress.http.middlewares.Middleware17.stripprefix.prefixes":                               "foobar, fiibar",
		"ingress.http.middlewares.Middleware17.stripprefix.forceslash":                             "true",
		"ingress.http.middlewares.Middleware18.stripprefixregex.regex":                             "foobar, fiibar",
		"ingress.http.middlewares.Middleware19.compress.encodings":                                 "foobar, fiibar",
		"ingress.http.middlewares.Middleware19.compress.minresponsebodybytes":                      "42",
		"ingress.http.middlewares.Middleware20.plugin.tomato.aaa":                                  "foo1",
		"ingress.http.middlewares.Middleware20.plugin.tomato.bbb":                                  "foo2",
		"ingress.http.routers.Router0.entrypoints":                                                 "foobar, fiibar",
		"ingress.http.routers.Router0.middlewares":                                                 "foobar, fiibar",
		"ingress.http.routers.Router0.priority":                                                    "42",
		"ingress.http.routers.Router0.rule":                                                        "foobar",
		"ingress.http.routers.Router0.tls":                                                         "true",
		"ingress.http.routers.Router0.service":                                                     "foobar",
		"ingress.http.routers.Router1.entrypoints":                                                 "foobar, fiibar",
		"ingress.http.routers.Router1.middlewares":                                                 "foobar, fiibar",
		"ingress.http.routers.Router1.priority":                                                    "42",
		"ingress.http.routers.Router1.rule":                                                        "foobar",
		"ingress.http.routers.Router1.service":                                                     "foobar",

		"ingress.http.services.Service0.loadbalancer.healthcheck.headers.name0":        "foobar",
		"ingress.http.services.Service0.loadbalancer.healthcheck.headers.name1":        "foobar",
		"ingress.http.services.Service0.loadbalancer.healthcheck.hostname":             "foobar",
		"ingress.http.services.Service0.loadbalancer.healthcheck.interval":             "1s",
		"ingress.http.services.Service0.loadbalancer.healthcheck.unhealthyinterval":    "1s",
		"ingress.http.services.Service0.loadbalancer.healthcheck.path":                 "foobar",
		"ingress.http.services.Service0.loadbalancer.healthcheck.method":               "foobar",
		"ingress.http.services.Service0.loadbalancer.healthcheck.status":               "401",
		"ingress.http.services.Service0.loadbalancer.healthcheck.port":                 "42",
		"ingress.http.services.Service0.loadbalancer.healthcheck.scheme":               "foobar",
		"ingress.http.services.Service0.loadbalancer.healthcheck.mode":                 "foobar",
		"ingress.http.services.Service0.loadbalancer.healthcheck.timeout":              "1s",
		"ingress.http.services.Service0.loadbalancer.healthcheck.followredirects":      "true",
		"ingress.http.services.Service0.loadbalancer.passhostheader":                   "true",
		"ingress.http.services.Service0.loadbalancer.responseforwarding.flushinterval": "1s",
		"ingress.http.services.Service0.loadbalancer.strategy":                         "foobar",
		"ingress.http.services.Service0.loadbalancer.server.url":                       "foobar",
		"ingress.http.services.Service0.loadbalancer.server.preservepath":              "true",
		"ingress.http.services.Service0.loadbalancer.server.scheme":                    "foobar",
		"ingress.http.services.Service0.loadbalancer.server.port":                      "8080",
		"ingress.http.services.Service0.loadbalancer.sticky.cookie.name":               "foobar",
		"ingress.http.services.Service0.loadbalancer.sticky.cookie.secure":             "true",
		"ingress.http.services.Service0.loadbalancer.sticky.cookie.path":               "/foobar",
		"ingress.http.services.Service0.loadbalancer.sticky.cookie.domain":             "foo.com",
		"ingress.http.services.Service0.loadbalancer.serversTransport":                 "foobar",
		"ingress.http.services.Service1.loadbalancer.healthcheck.headers.name0":        "foobar",
		"ingress.http.services.Service1.loadbalancer.healthcheck.headers.name1":        "foobar",
		"ingress.http.services.Service1.loadbalancer.healthcheck.hostname":             "foobar",
		"ingress.http.services.Service1.loadbalancer.healthcheck.interval":             "1s",
		"ingress.http.services.Service1.loadbalancer.healthcheck.unhealthyinterval":    "1s",
		"ingress.http.services.Service1.loadbalancer.healthcheck.path":                 "foobar",
		"ingress.http.services.Service1.loadbalancer.healthcheck.method":               "foobar",
		"ingress.http.services.Service1.loadbalancer.healthcheck.status":               "401",
		"ingress.http.services.Service1.loadbalancer.healthcheck.port":                 "42",
		"ingress.http.services.Service1.loadbalancer.healthcheck.scheme":               "foobar",
		"ingress.http.services.Service1.loadbalancer.healthcheck.mode":                 "foobar",
		"ingress.http.services.Service1.loadbalancer.healthcheck.timeout":              "1s",
		"ingress.http.services.Service1.loadbalancer.healthcheck.followredirects":      "true",
		"ingress.http.services.Service1.loadbalancer.passhostheader":                   "true",
		"ingress.http.services.Service1.loadbalancer.responseforwarding.flushinterval": "1s",
		"ingress.http.services.Service1.loadbalancer.strategy":                         "foobar",
		"ingress.http.services.Service1.loadbalancer.server.url":                       "foobar",
		"ingress.http.services.Service1.loadbalancer.server.preservepath":              "true",
		"ingress.http.services.Service1.loadbalancer.server.scheme":                    "foobar",
		"ingress.http.services.Service1.loadbalancer.server.port":                      "8080",
		"ingress.http.services.Service1.loadbalancer.sticky":                           "false",
		"ingress.http.services.Service1.loadbalancer.sticky.cookie.name":               "fui",
		"ingress.http.services.Service1.loadbalancer.serversTransport":                 "foobar",

		"ingress.tcp.middlewares.Middleware0.ipallowlist.sourcerange":      "foobar, fiibar",
		"ingress.tcp.middlewares.Middleware2.inflightconn.amount":          "42",
		"ingress.tcp.routers.Router0.rule":                                 "foobar",
		"ingress.tcp.routers.Router0.priority":                             "42",
		"ingress.tcp.routers.Router0.entrypoints":                          "foobar, fiibar",
		"ingress.tcp.routers.Router0.service":                              "foobar",
		"ingress.tcp.routers.Router0.tls.passthrough":                      "false",
		"ingress.tcp.routers.Router0.tls.options":                          "foo",
		"ingress.tcp.routers.Router1.rule":                                 "foobar",
		"ingress.tcp.routers.Router1.priority":                             "42",
		"ingress.tcp.routers.Router1.entrypoints":                          "foobar, fiibar",
		"ingress.tcp.routers.Router1.service":                              "foobar",
		"ingress.tcp.routers.Router1.tls.options":                          "foo",
		"ingress.tcp.routers.Router1.tls.passthrough":                      "false",
		"ingress.tcp.services.Service0.loadbalancer.server.Port":           "42",
		"ingress.tcp.services.Service0.loadbalancer.TerminationDelay":      "42",
		"ingress.tcp.services.Service0.loadbalancer.proxyProtocol.version": "42",
		"ingress.tcp.services.Service0.loadbalancer.serversTransport":      "foo",
		"ingress.tcp.services.Service1.loadbalancer.server.Port":           "42",
		"ingress.tcp.services.Service1.loadbalancer.TerminationDelay":      "42",
		"ingress.tcp.services.Service1.loadbalancer.proxyProtocol":         "true",
		"ingress.tcp.services.Service1.loadbalancer.serversTransport":      "foo",

		"ingress.udp.routers.Router0.entrypoints":                "foobar, fiibar",
		"ingress.udp.routers.Router0.service":                    "foobar",
		"ingress.udp.routers.Router1.entrypoints":                "foobar, fiibar",
		"ingress.udp.routers.Router1.service":                    "foobar",
		"ingress.udp.services.Service0.loadbalancer.server.Port": "42",
		"ingress.udp.services.Service1.loadbalancer.server.Port": "42",

		"ingress.tls.stores.default.defaultgeneratedcert.resolver":    "foobar",
		"ingress.tls.stores.default.defaultgeneratedcert.domain.main": "foobar",
		"ingress.tls.stores.default.defaultgeneratedcert.domain.sans": "foobar, fiibar",
	}

	configuration, err := DecodeConfiguration(labels)
	require.NoError(t, err)

	expected := &dynamic.Configuration{
		TCP: &dynamic.TCPConfiguration{
			Routers: map[string]*dynamic.TCPRouter{
				"Router0": {
					EntryPoints: []string{
						"foobar",
						"fiibar",
					},
					Service:  "foobar",
					Rule:     "foobar",
					Priority: 42,
					TLS: &dynamic.RouterTCPTLSConfig{
						Passthrough: false,
						Options:     "foo",
					},
				},
				"Router1": {
					EntryPoints: []string{
						"foobar",
						"fiibar",
					},
					Service:  "foobar",
					Rule:     "foobar",
					Priority: 42,
					TLS: &dynamic.RouterTCPTLSConfig{
						Passthrough: false,
						Options:     "foo",
					},
				},
			},
			Middlewares: map[string]*dynamic.TCPMiddleware{
				"Middleware0": {
					IPAllowList: &dynamic.TCPIPAllowList{
						SourceRange: []string{"foobar", "fiibar"},
					},
				},
				"Middleware2": {
					InFlightConn: &dynamic.TCPInFlightConn{
						Amount: 42,
					},
				},
			},
			Services: map[string]*dynamic.TCPService{
				"Service0": {
					LoadBalancer: &dynamic.TCPServersLoadBalancer{
						Servers: []dynamic.TCPServer{
							{
								Port: "42",
							},
						},
						TerminationDelay: pointer(42),
						ProxyProtocol:    &dynamic.ProxyProtocol{Version: 42},
						ServersTransport: "foo",
					},
				},
				"Service1": {
					LoadBalancer: &dynamic.TCPServersLoadBalancer{
						Servers: []dynamic.TCPServer{
							{
								Port: "42",
							},
						},
						TerminationDelay: pointer(42),
						ProxyProtocol:    &dynamic.ProxyProtocol{Version: 2},
						ServersTransport: "foo",
					},
				},
			},
		},
		UDP: &dynamic.UDPConfiguration{
			Routers: map[string]*dynamic.UDPRouter{
				"Router0": {
					EntryPoints: []string{
						"foobar",
						"fiibar",
					},
					Service: "foobar",
				},
				"Router1": {
					EntryPoints: []string{
						"foobar",
						"fiibar",
					},
					Service: "foobar",
				},
			},
			Services: map[string]*dynamic.UDPService{
				"Service0": {
					LoadBalancer: &dynamic.UDPServersLoadBalancer{
						Servers: []dynamic.UDPServer{
							{
								Port: "42",
							},
						},
					},
				},
				"Service1": {
					LoadBalancer: &dynamic.UDPServersLoadBalancer{
						Servers: []dynamic.UDPServer{
							{
								Port: "42",
							},
						},
					},
				},
			},
		},
		HTTP: &dynamic.HTTPConfiguration{
			Routers: map[string]*dynamic.Router{
				"Router0": {
					EntryPoints: []string{
						"foobar",
						"fiibar",
					},
					Middlewares: []string{
						"foobar",
						"fiibar",
					},
					Service:  "foobar",
					Rule:     "foobar",
					Priority: 42,
					TLS:      &dynamic.RouterTLSConfig{},
				},
				"Router1": {
					EntryPoints: []string{
						"foobar",
						"fiibar",
					},
					Middlewares: []string{
						"foobar",
						"fiibar",
					},
					Service:  "foobar",
					Rule:     "foobar",
					Priority: 42,
				},
			},
			Middlewares: map[string]*dynamic.Middleware{
				"Middleware0": {
					AddPrefix: &dynamic.AddPrefix{
						Prefix: "foobar",
					},
				},
				"Middleware1": {
					BasicAuth: &dynamic.BasicAuth{
						Users: []string{
							"foobar",
							"fiibar",
						},
						UsersFile:    "foobar",
						Realm:        "foobar",
						RemoveHeader: true,
						HeaderField:  "foobar",
					},
				},
				"Middleware10": {
					InFlightReq: &dynamic.InFlightReq{
						Amount: 42,
						SourceCriterion: &dynamic.SourceCriterion{
							IPStrategy: &dynamic.IPStrategy{
								Depth:       42,
								ExcludedIPs: []string{"foobar", "fiibar"},
								IPv6Subnet:  intPtr(42),
							},
							RequestHeaderName: "foobar",
							RequestHost:       true,
						},
					},
				},
				"Middleware11": {
					PassTLSClientCert: &dynamic.PassTLSClientCert{
						PEM: true,
						Info: &dynamic.TLSClientCertificateInfo{
							NotAfter:     true,
							NotBefore:    true,
							SerialNumber: true,
							Subject: &dynamic.TLSClientCertificateSubjectDNInfo{
								Country:            true,
								Province:           true,
								Locality:           true,
								Organization:       true,
								OrganizationalUnit: true,
								CommonName:         true,
								SerialNumber:       true,
								DomainComponent:    true,
							},
							Issuer: &dynamic.TLSClientCertificateIssuerDNInfo{
								Country:         true,
								Province:        true,
								Locality:        true,
								Organization:    true,
								CommonName:      true,
								SerialNumber:    true,
								DomainComponent: true,
							},
							Sans: true,
						},
					},
				},
				"Middleware12": {
					RateLimit: &dynamic.RateLimit{
						Average: 42,
						Burst:   42,
						Period:  ptypes.Duration(time.Second),
						SourceCriterion: &dynamic.SourceCriterion{
							IPStrategy: &dynamic.IPStrategy{
								Depth:       42,
								ExcludedIPs: []string{"foobar", "foobar"},
								IPv6Subnet:  intPtr(42),
							},
							RequestHeaderName: "foobar",
							RequestHost:       true,
						},
					},
				},
				"Middleware13": {
					RedirectRegex: &dynamic.RedirectRegex{
						Regex:       "foobar",
						Replacement: "foobar",
						Permanent:   true,
					},
				},
				"Middleware13b": {
					RedirectScheme: &dynamic.RedirectScheme{
						Scheme:    "https",
						Port:      "80",
						Permanent: true,
					},
				},
				"Middleware14": {
					ReplacePath: &dynamic.ReplacePath{
						Path: "foobar",
					},
				},
				"Middleware15": {
					ReplacePathRegex: &dynamic.ReplacePathRegex{
						Regex:       "foobar",
						Replacement: "foobar",
					},
				},
				"Middleware16": {
					Retry: &dynamic.Retry{
						Attempts:                   42,
						InitialInterval:            ptypes.Duration(time.Second),
						Timeout:                    ptypes.Duration(time.Second),
						MaxRequestBodyBytes:        pointer[int64](42),
						Status:                     []string{"foobar", "foobar"},
						DisableRetryOnNetworkError: true,
						RetryNonIdempotentMethod:   true,
					},
				},
				"Middleware17": {
					StripPrefix: &dynamic.StripPrefix{
						Prefixes: []string{
							"foobar",
							"fiibar",
						},
						ForceSlash: pointer(true),
					},
				},
				"Middleware18": {
					StripPrefixRegex: &dynamic.StripPrefixRegex{
						Regex: []string{
							"foobar",
							"fiibar",
						},
					},
				},
				"Middleware19": {
					Compress: &dynamic.Compress{
						MinResponseBodyBytes: 42,
						Encodings: []string{
							"foobar",
							"fiibar",
						},
					},
				},
				"Middleware2": {
					Buffering: &dynamic.Buffering{
						MaxRequestBodyBytes:  42,
						MemRequestBodyBytes:  42,
						MaxResponseBodyBytes: 42,
						MemResponseBodyBytes: 42,
						RetryExpression:      "foobar",
					},
				},
				"Middleware3": {
					Chain: &dynamic.Chain{
						Middlewares: []string{
							"foobar",
							"fiibar",
						},
					},
				},
				"Middleware4": {
					CircuitBreaker: &dynamic.CircuitBreaker{
						Expression:       "foobar",
						CheckPeriod:      ptypes.Duration(time.Second),
						FallbackDuration: ptypes.Duration(time.Second),
						RecoveryDuration: ptypes.Duration(time.Second),
						ResponseCode:     403,
					},
				},
				"Middleware5": {
					DigestAuth: &dynamic.DigestAuth{
						Users: []string{
							"foobar",
							"fiibar",
						},
						UsersFile:    "foobar",
						RemoveHeader: true,
						Realm:        "foobar",
						HeaderField:  "foobar",
					},
				},
				"Middleware6": {
					Errors: &dynamic.ErrorPage{
						Status: []string{
							"foobar",
							"fiibar",
						},
						Service: "foobar",
						Query:   "foobar",
					},
				},
				"Middleware7": {
					ForwardAuth: &dynamic.ForwardAuth{
						Address: "foobar",
						TLS: &dynamic.ClientTLS{
							CA:                 "foobar",
							Cert:               "foobar",
							Key:                "foobar",
							InsecureSkipVerify: true,
							CAOptional:         pointer(true),
						},
						TrustForwardHeader: true,
						AuthResponseHeaders: []string{
							"foobar",
							"fiibar",
						},
						AuthRequestHeaders: []string{
							"foobar",
							"fiibar",
						},
						ForwardBody:           true,
						MaxBodySize:           pointer(int64(42)),
						PreserveRequestMethod: true,
						MaxResponseBodySize:   pointer[int64](42),
					},
				},
				"Middleware8": {
					Headers: &dynamic.Headers{
						CustomRequestHeaders: map[string]string{
							"name0": "foobar",
							"name1": "foobar",
						},
						CustomResponseHeaders: map[string]string{
							"name0": "foobar",
							"name1": "foobar",
						},
						AccessControlAllowCredentials: true,
						AccessControlAllowHeaders: []string{
							"X-foobar",
							"X-fiibar",
						},
						AccessControlAllowMethods: []string{
							"GET",
							"PUT",
						},
						AccessControlAllowOriginList: []string{
							"foobar",
							"fiibar",
						},
						AccessControlAllowOriginListRegex: []string{
							"foobar",
							"fiibar",
						},
						AccessControlExposeHeaders: []string{
							"X-foobar",
							"X-fiibar",
						},
						AccessControlMaxAge: 200,
						AddVaryHeader:       true,
						AllowedHosts: []string{
							"foobar",
							"fiibar",
						},
						HostsProxyHeaders: []string{
							"foobar",
							"fiibar",
						},
						SSLRedirect:          pointer(true),
						SSLTemporaryRedirect: pointer(true),
						SSLHost:              pointer("foobar"),
						SSLProxyHeaders: map[string]string{
							"name0": "foobar",
							"name1": "foobar",
						},
						SSLForceHost:                    pointer(true),
						STSSeconds:                      42,
						STSIncludeSubdomains:            true,
						STSPreload:                      true,
						ForceSTSHeader:                  true,
						FrameDeny:                       true,
						CustomFrameOptionsValue:         "foobar",
						ContentTypeNosniff:              true,
						BrowserXSSFilter:                true,
						CustomBrowserXSSValue:           "foobar",
						ContentSecurityPolicy:           "foobar",
						ContentSecurityPolicyReportOnly: "foobar",
						PublicKey:                       "foobar",
						ReferrerPolicy:                  "foobar",
						FeaturePolicy:                   pointer("foobar"),
						PermissionsPolicy:               "foobar",
						IsDevelopment:                   true,
					},
				},
				"Middleware9": {
					IPAllowList: &dynamic.IPAllowList{
						SourceRange: []string{
							"foobar",
							"fiibar",
						},
						IPStrategy: &dynamic.IPStrategy{
							Depth: 42,
							ExcludedIPs: []string{
								"foobar",
								"fiibar",
							},
							IPv6Subnet: intPtr(42),
						},
					},
				},
				"Middleware20": {
					Plugin: map[string]dynamic.PluginConf{
						"tomato": {
							"aaa": "foo1",
							"bbb": "foo2",
						},
					},
				},
			},
			Services: map[string]*dynamic.Service{
				"Service0": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Strategy: "foobar",
						Sticky: &dynamic.Sticky{
							Cookie: &dynamic.Cookie{
								Name:     "foobar",
								Secure:   true,
								HTTPOnly: false,
								Domain:   "foo.com",
								Path:     func(v string) *string { return &v }("/foobar"),
							},
						},
						Servers: []dynamic.Server{
							{
								URL:          "foobar",
								PreservePath: true,
								Scheme:       "foobar",
								Port:         "8080",
							},
						},
						HealthCheck: &dynamic.ServerHealthCheck{
							Scheme:            "foobar",
							Mode:              "foobar",
							Path:              "foobar",
							Method:            "foobar",
							Status:            401,
							Port:              42,
							Interval:          ptypes.Duration(time.Second),
							UnhealthyInterval: pointer(ptypes.Duration(time.Second)),
							Timeout:           ptypes.Duration(time.Second),
							Hostname:          "foobar",
							Headers: map[string]string{
								"name0": "foobar",
								"name1": "foobar",
							},
							FollowRedirects: pointer(true),
						},
						PassHostHeader: pointer(true),
						ResponseForwarding: &dynamic.ResponseForwarding{
							FlushInterval: ptypes.Duration(time.Second),
						},
						ServersTransport: "foobar",
					},
				},
				"Service1": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Strategy: "foobar",
						Servers: []dynamic.Server{
							{
								URL:          "foobar",
								PreservePath: true,
								Scheme:       "foobar",
								Port:         "8080",
							},
						},
						HealthCheck: &dynamic.ServerHealthCheck{
							Scheme:            "foobar",
							Mode:              "foobar",
							Path:              "foobar",
							Method:            "foobar",
							Status:            401,
							Port:              42,
							Interval:          ptypes.Duration(time.Second),
							UnhealthyInterval: pointer(ptypes.Duration(time.Second)),
							Timeout:           ptypes.Duration(time.Second),
							Hostname:          "foobar",
							Headers: map[string]string{
								"name0": "foobar",
								"name1": "foobar",
							},
							FollowRedirects: pointer(true),
						},
						PassHostHeader: pointer(true),
						ResponseForwarding: &dynamic.ResponseForwarding{
							FlushInterval: ptypes.Duration(time.Second),
						},
						ServersTransport: "foobar",
					},
				},
			},
		},
		TLS: &dynamic.TLSConfiguration{
			Stores: map[string]tls.Store{
				"default": {
					DefaultGeneratedCert: &tls.GeneratedCert{
						Resolver: "foobar",
						Domain: &types.Domain{
							Main: "foobar",
							SANs: []string{"foobar", "fiibar"},
						},
					},
				},
			},
		},
	}

	assert.Nil(t, configuration.HTTP.ServersTransports)
	assert.Nil(t, configuration.TCP.ServersTransports)
	assert.Equal(t, expected, configuration)
}

func TestEncodeConfiguration(t *testing.T) {
	configuration := &dynamic.Configuration{
		TCP: &dynamic.TCPConfiguration{
			Routers: map[string]*dynamic.TCPRouter{
				"Router0": {
					EntryPoints: []string{
						"foobar",
						"fiibar",
					},
					Service:  "foobar",
					Rule:     "foobar",
					Priority: 42,
					TLS: &dynamic.RouterTCPTLSConfig{
						Passthrough: false,
						Options:     "foo",
					},
				},
				"Router1": {
					EntryPoints: []string{
						"foobar",
						"fiibar",
					},
					Service:  "foobar",
					Rule:     "foobar",
					Priority: 42,
					TLS: &dynamic.RouterTCPTLSConfig{
						Passthrough: false,
						Options:     "foo",
					},
				},
			},
			Middlewares: map[string]*dynamic.TCPMiddleware{
				"Middleware0": {
					IPAllowList: &dynamic.TCPIPAllowList{
						SourceRange: []string{"foobar", "fiibar"},
					},
				},
				"Middleware2": {
					InFlightConn: &dynamic.TCPInFlightConn{
						Amount: 42,
					},
				},
			},
			Services: map[string]*dynamic.TCPService{
				"Service0": {
					LoadBalancer: &dynamic.TCPServersLoadBalancer{
						Servers: []dynamic.TCPServer{
							{
								Port: "42",
							},
						},
						ServersTransport: "foo",
						TerminationDelay: pointer(42),
					},
				},
				"Service1": {
					LoadBalancer: &dynamic.TCPServersLoadBalancer{
						Servers: []dynamic.TCPServer{
							{
								Port: "42",
							},
						},
						ServersTransport: "foo",
						TerminationDelay: pointer(42),
					},
				},
			},
		},
		UDP: &dynamic.UDPConfiguration{
			Routers: map[string]*dynamic.UDPRouter{
				"Router0": {
					EntryPoints: []string{
						"foobar",
						"fiibar",
					},
					Service: "foobar",
				},
				"Router1": {
					EntryPoints: []string{
						"foobar",
						"fiibar",
					},
					Service: "foobar",
				},
			},
			Services: map[string]*dynamic.UDPService{
				"Service0": {
					LoadBalancer: &dynamic.UDPServersLoadBalancer{
						Servers: []dynamic.UDPServer{
							{
								Port: "42",
							},
						},
					},
				},
				"Service1": {
					LoadBalancer: &dynamic.UDPServersLoadBalancer{
						Servers: []dynamic.UDPServer{
							{
								Port: "42",
							},
						},
					},
				},
			},
		},
		HTTP: &dynamic.HTTPConfiguration{
			Routers: map[string]*dynamic.Router{
				"Router0": {
					EntryPoints: []string{
						"foobar",
						"fiibar",
					},
					Middlewares: []string{
						"foobar",
						"fiibar",
					},
					Service:  "foobar",
					Rule:     "foobar",
					Priority: 42,
					TLS:      &dynamic.RouterTLSConfig{},
					Observability: &dynamic.RouterObservabilityConfig{
						AccessLogs: pointer(true),
						Tracing:    pointer(true),
						Metrics:    pointer(true),
					},
				},
				"Router1": {
					EntryPoints: []string{
						"foobar",
						"fiibar",
					},
					Middlewares: []string{
						"foobar",
						"fiibar",
					},
					Service:  "foobar",
					Rule:     "foobar",
					Priority: 42,
					Observability: &dynamic.RouterObservabilityConfig{
						AccessLogs: pointer(true),
						Tracing:    pointer(true),
						Metrics:    pointer(true),
					},
				},
			},
			Middlewares: map[string]*dynamic.Middleware{
				"Middleware0": {
					AddPrefix: &dynamic.AddPrefix{
						Prefix: "foobar",
					},
				},
				"Middleware1": {
					BasicAuth: &dynamic.BasicAuth{
						Users: []string{
							"foobar",
							"fiibar",
						},
						UsersFile:    "foobar",
						Realm:        "foobar",
						RemoveHeader: true,
						HeaderField:  "foobar",
					},
				},
				"Middleware10": {
					InFlightReq: &dynamic.InFlightReq{
						Amount: 42,
						SourceCriterion: &dynamic.SourceCriterion{
							IPStrategy: &dynamic.IPStrategy{
								Depth:       42,
								ExcludedIPs: []string{"foobar", "fiibar"},
								IPv6Subnet:  intPtr(42),
							},
							RequestHeaderName: "foobar",
							RequestHost:       true,
						},
					},
				},
				"Middleware11": {
					PassTLSClientCert: &dynamic.PassTLSClientCert{
						PEM: true,
						Info: &dynamic.TLSClientCertificateInfo{
							NotAfter:     true,
							NotBefore:    true,
							SerialNumber: true,
							Subject: &dynamic.TLSClientCertificateSubjectDNInfo{
								Country:            true,
								Province:           true,
								Locality:           true,
								Organization:       true,
								OrganizationalUnit: true,
								CommonName:         true,
								SerialNumber:       true,
								DomainComponent:    true,
							},
							Issuer: &dynamic.TLSClientCertificateIssuerDNInfo{
								Country:         true,
								Province:        true,
								Locality:        true,
								Organization:    true,
								CommonName:      true,
								SerialNumber:    true,
								DomainComponent: true,
							}, Sans: true,
						},
					},
				},
				"Middleware12": {
					RateLimit: &dynamic.RateLimit{
						Average: 42,
						Burst:   42,
						Period:  ptypes.Duration(time.Second),
						SourceCriterion: &dynamic.SourceCriterion{
							IPStrategy: &dynamic.IPStrategy{
								Depth:       42,
								ExcludedIPs: []string{"foobar", "foobar"},
								IPv6Subnet:  intPtr(42),
							},
							RequestHeaderName: "foobar",
							RequestHost:       true,
						},
					},
				},
				"Middleware13": {
					RedirectRegex: &dynamic.RedirectRegex{
						Regex:       "foobar",
						Replacement: "foobar",
						Permanent:   true,
					},
				},
				"Middleware13b": {
					RedirectScheme: &dynamic.RedirectScheme{
						Scheme:    "https",
						Port:      "80",
						Permanent: true,
					},
				},
				"Middleware14": {
					ReplacePath: &dynamic.ReplacePath{
						Path: "foobar",
					},
				},
				"Middleware15": {
					ReplacePathRegex: &dynamic.ReplacePathRegex{
						Regex:       "foobar",
						Replacement: "foobar",
					},
				},
				"Middleware16": {
					Retry: &dynamic.Retry{
						Attempts:                   42,
						InitialInterval:            ptypes.Duration(time.Second),
						Timeout:                    ptypes.Duration(time.Second),
						MaxRequestBodyBytes:        pointer[int64](42),
						Status:                     []string{"foobar", "foobar"},
						DisableRetryOnNetworkError: true,
						RetryNonIdempotentMethod:   true,
					},
				},
				"Middleware17": {
					StripPrefix: &dynamic.StripPrefix{
						Prefixes: []string{
							"foobar",
							"fiibar",
						},
						ForceSlash: pointer(true),
					},
				},
				"Middleware18": {
					StripPrefixRegex: &dynamic.StripPrefixRegex{
						Regex: []string{
							"foobar",
							"fiibar",
						},
					},
				},
				"Middleware19": {
					Compress: &dynamic.Compress{
						MinResponseBodyBytes: 42,
						Encodings: []string{
							"foobar",
							"fiibar",
						},
					},
				},
				"Middleware2": {
					Buffering: &dynamic.Buffering{
						MaxRequestBodyBytes:  42,
						MemRequestBodyBytes:  42,
						MaxResponseBodyBytes: 42,
						MemResponseBodyBytes: 42,
						RetryExpression:      "foobar",
					},
				},
				"Middleware20": {
					Plugin: map[string]dynamic.PluginConf{
						"tomato": {
							"aaa": "foo1",
							"bbb": "foo2",
						},
					},
				},
				"Middleware3": {
					Chain: &dynamic.Chain{
						Middlewares: []string{
							"foobar",
							"fiibar",
						},
					},
				},
				"Middleware4": {
					CircuitBreaker: &dynamic.CircuitBreaker{
						Expression:       "foobar",
						CheckPeriod:      ptypes.Duration(time.Second),
						FallbackDuration: ptypes.Duration(time.Second),
						RecoveryDuration: ptypes.Duration(time.Second),
						ResponseCode:     404,
					},
				},
				"Middleware5": {
					DigestAuth: &dynamic.DigestAuth{
						Users: []string{
							"foobar",
							"fiibar",
						},
						UsersFile:    "foobar",
						RemoveHeader: true,
						Realm:        "foobar",
						HeaderField:  "foobar",
					},
				},
				"Middleware6": {
					Errors: &dynamic.ErrorPage{
						Status: []string{
							"foobar",
							"fiibar",
						},
						Service: "foobar",
						Query:   "foobar",
					},
				},
				"Middleware7": {
					ForwardAuth: &dynamic.ForwardAuth{
						Address: "foobar",
						TLS: &dynamic.ClientTLS{
							CA:                 "foobar",
							Cert:               "foobar",
							Key:                "foobar",
							InsecureSkipVerify: true,
							CAOptional:         pointer(true),
						},
						TrustForwardHeader: true,
						AuthResponseHeaders: []string{
							"foobar",
							"fiibar",
						},
						AuthRequestHeaders: []string{
							"foobar",
							"fiibar",
						},
						ForwardBody:           true,
						MaxBodySize:           pointer(int64(42)),
						PreserveRequestMethod: true,
						MaxResponseBodySize:   pointer[int64](42),
					},
				},
				"Middleware8": {
					Headers: &dynamic.Headers{
						CustomRequestHeaders: map[string]string{
							"name0": "foobar",
							"name1": "foobar",
						},
						CustomResponseHeaders: map[string]string{
							"name0": "foobar",
							"name1": "foobar",
						},
						AccessControlAllowCredentials: true,
						AccessControlAllowHeaders: []string{
							"X-foobar",
							"X-fiibar",
						},
						AccessControlAllowMethods: []string{
							"GET",
							"PUT",
						},
						AccessControlAllowOriginList: []string{
							"foobar",
							"fiibar",
						},
						AccessControlAllowOriginListRegex: []string{
							"foobar",
							"fiibar",
						},
						AccessControlExposeHeaders: []string{
							"X-foobar",
							"X-fiibar",
						},
						AccessControlMaxAge: 200,
						AddVaryHeader:       true,
						AllowedHosts: []string{
							"foobar",
							"fiibar",
						},
						HostsProxyHeaders: []string{
							"foobar",
							"fiibar",
						},
						SSLRedirect:          pointer(true),
						SSLTemporaryRedirect: pointer(true),
						SSLHost:              pointer("foobar"),
						SSLProxyHeaders: map[string]string{
							"name0": "foobar",
							"name1": "foobar",
						},
						SSLForceHost:                    pointer(true),
						STSSeconds:                      42,
						STSIncludeSubdomains:            true,
						STSPreload:                      true,
						ForceSTSHeader:                  true,
						FrameDeny:                       true,
						CustomFrameOptionsValue:         "foobar",
						ContentTypeNosniff:              true,
						BrowserXSSFilter:                true,
						CustomBrowserXSSValue:           "foobar",
						ContentSecurityPolicy:           "foobar",
						ContentSecurityPolicyReportOnly: "foobar",
						PublicKey:                       "foobar",
						ReferrerPolicy:                  "foobar",
						FeaturePolicy:                   pointer("foobar"),
						PermissionsPolicy:               "foobar",
						IsDevelopment:                   true,
					},
				},
				"Middleware9": {
					IPAllowList: &dynamic.IPAllowList{
						SourceRange: []string{
							"foobar",
							"fiibar",
						},
						IPStrategy: &dynamic.IPStrategy{
							Depth: 42,
							ExcludedIPs: []string{
								"foobar",
								"fiibar",
							},
							IPv6Subnet: intPtr(42),
						},
					},
				},
			},
			Services: map[string]*dynamic.Service{
				"Service0": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Strategy: "foobar",
						Sticky: &dynamic.Sticky{
							Cookie: &dynamic.Cookie{
								Name:     "foobar",
								HTTPOnly: true,
								Domain:   "foo.com",
								Path:     func(v string) *string { return &v }("/foobar"),
							},
						},
						Servers: []dynamic.Server{
							{
								URL:          "foobar",
								PreservePath: true,
								Scheme:       "foobar",
								Port:         "8080",
							},
						},
						HealthCheck: &dynamic.ServerHealthCheck{
							Scheme:            "foobar",
							Path:              "foobar",
							Method:            "foobar",
							Status:            401,
							Port:              42,
							Interval:          ptypes.Duration(time.Second),
							UnhealthyInterval: pointer(ptypes.Duration(time.Second)),
							Timeout:           ptypes.Duration(time.Second),
							Hostname:          "foobar",
							Headers: map[string]string{
								"name0": "foobar",
								"name1": "foobar",
							},
						},
						PassHostHeader: pointer(true),
						ResponseForwarding: &dynamic.ResponseForwarding{
							FlushInterval: ptypes.Duration(time.Second),
						},
						ServersTransport: "foobar",
					},
				},
				"Service1": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Strategy: "foobar",
						Servers: []dynamic.Server{
							{
								URL:          "foobar",
								PreservePath: true,
								Scheme:       "foobar",
								Port:         "8080",
							},
						},
						HealthCheck: &dynamic.ServerHealthCheck{
							Scheme:            "foobar",
							Path:              "foobar",
							Method:            "foobar",
							Status:            401,
							Port:              42,
							Interval:          ptypes.Duration(time.Second),
							UnhealthyInterval: pointer(ptypes.Duration(time.Second)),
							Timeout:           ptypes.Duration(time.Second),
							Hostname:          "foobar",
							Headers: map[string]string{
								"name0": "foobar",
								"name1": "foobar",
							},
						},
						PassHostHeader: pointer(true),
						ResponseForwarding: &dynamic.ResponseForwarding{
							FlushInterval: ptypes.Duration(time.Second),
						},
						ServersTransport: "foobar",
					},
				},
			},
		},
		TLS: &dynamic.TLSConfiguration{
			Stores: map[string]tls.Store{
				"default": {
					DefaultGeneratedCert: &tls.GeneratedCert{
						Resolver: "foobar",
						Domain: &types.Domain{
							Main: "foobar",
							SANs: []string{"foobar", "fiibar"},
						},
					},
				},
			},
		},
	}

	labels, err := EncodeConfiguration(configuration)
	require.NoError(t, err)

	expected := map[string]string{
		"ingress.HTTP.Middlewares.Middleware0.AddPrefix.Prefix":                                    "foobar",
		"ingress.HTTP.Middlewares.Middleware1.BasicAuth.HeaderField":                               "foobar",
		"ingress.HTTP.Middlewares.Middleware1.BasicAuth.Realm":                                     "foobar",
		"ingress.HTTP.Middlewares.Middleware1.BasicAuth.RemoveHeader":                              "true",
		"ingress.HTTP.Middlewares.Middleware1.BasicAuth.Users":                                     "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware1.BasicAuth.UsersFile":                                 "foobar",
		"ingress.HTTP.Middlewares.Middleware2.Buffering.MaxRequestBodyBytes":                       "42",
		"ingress.HTTP.Middlewares.Middleware2.Buffering.MaxResponseBodyBytes":                      "42",
		"ingress.HTTP.Middlewares.Middleware2.Buffering.MemRequestBodyBytes":                       "42",
		"ingress.HTTP.Middlewares.Middleware2.Buffering.MemResponseBodyBytes":                      "42",
		"ingress.HTTP.Middlewares.Middleware2.Buffering.RetryExpression":                           "foobar",
		"ingress.HTTP.Middlewares.Middleware3.Chain.Middlewares":                                   "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware4.CircuitBreaker.Expression":                           "foobar",
		"ingress.HTTP.Middlewares.Middleware4.CircuitBreaker.CheckPeriod":                          "1000000000",
		"ingress.HTTP.Middlewares.Middleware4.CircuitBreaker.FallbackDuration":                     "1000000000",
		"ingress.HTTP.Middlewares.Middleware4.CircuitBreaker.RecoveryDuration":                     "1000000000",
		"ingress.HTTP.Middlewares.Middleware4.CircuitBreaker.ResponseCode":                         "404",
		"ingress.HTTP.Middlewares.Middleware5.DigestAuth.HeaderField":                              "foobar",
		"ingress.HTTP.Middlewares.Middleware5.DigestAuth.Realm":                                    "foobar",
		"ingress.HTTP.Middlewares.Middleware5.DigestAuth.RemoveHeader":                             "true",
		"ingress.HTTP.Middlewares.Middleware5.DigestAuth.Users":                                    "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware5.DigestAuth.UsersFile":                                "foobar",
		"ingress.HTTP.Middlewares.Middleware6.Errors.Query":                                        "foobar",
		"ingress.HTTP.Middlewares.Middleware6.Errors.Service":                                      "foobar",
		"ingress.HTTP.Middlewares.Middleware6.Errors.Status":                                       "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.Address":                                 "foobar",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.AuthResponseHeaders":                     "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.AuthRequestHeaders":                      "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.ForwardBody":                             "true",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.MaxBodySize":                             "42",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.TLS.CA":                                  "foobar",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.TLS.CAOptional":                          "true",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.TLS.Cert":                                "foobar",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.TLS.InsecureSkipVerify":                  "true",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.TLS.Key":                                 "foobar",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.TrustForwardHeader":                      "true",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.PreserveLocationHeader":                  "false",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.PreserveRequestMethod":                   "true",
		"ingress.HTTP.Middlewares.Middleware7.ForwardAuth.MaxResponseBodySize":                     "42",
		"ingress.HTTP.Middlewares.Middleware8.Headers.AccessControlAllowCredentials":               "true",
		"ingress.HTTP.Middlewares.Middleware8.Headers.AccessControlAllowHeaders":                   "X-foobar, X-fiibar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.AccessControlAllowMethods":                   "GET, PUT",
		"ingress.HTTP.Middlewares.Middleware8.Headers.AccessControlAllowOriginList":                "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.AccessControlAllowOriginListRegex":           "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.AccessControlExposeHeaders":                  "X-foobar, X-fiibar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.AccessControlMaxAge":                         "200",
		"ingress.HTTP.Middlewares.Middleware8.Headers.AddVaryHeader":                               "true",
		"ingress.HTTP.Middlewares.Middleware8.Headers.AllowedHosts":                                "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.BrowserXSSFilter":                            "true",
		"ingress.HTTP.Middlewares.Middleware8.Headers.ContentSecurityPolicy":                       "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.ContentSecurityPolicyReportOnly":             "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.ContentTypeNosniff":                          "true",
		"ingress.HTTP.Middlewares.Middleware8.Headers.CustomBrowserXSSValue":                       "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.CustomFrameOptionsValue":                     "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.CustomRequestHeaders.name0":                  "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.CustomRequestHeaders.name1":                  "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.CustomResponseHeaders.name0":                 "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.CustomResponseHeaders.name1":                 "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.ForceSTSHeader":                              "true",
		"ingress.HTTP.Middlewares.Middleware8.Headers.FrameDeny":                                   "true",
		"ingress.HTTP.Middlewares.Middleware8.Headers.HostsProxyHeaders":                           "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.IsDevelopment":                               "true",
		"ingress.HTTP.Middlewares.Middleware8.Headers.PublicKey":                                   "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.ReferrerPolicy":                              "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.FeaturePolicy":                               "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.PermissionsPolicy":                           "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.SSLForceHost":                                "true",
		"ingress.HTTP.Middlewares.Middleware8.Headers.SSLHost":                                     "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.SSLProxyHeaders.name0":                       "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.SSLProxyHeaders.name1":                       "foobar",
		"ingress.HTTP.Middlewares.Middleware8.Headers.SSLRedirect":                                 "true",
		"ingress.HTTP.Middlewares.Middleware8.Headers.SSLTemporaryRedirect":                        "true",
		"ingress.HTTP.Middlewares.Middleware8.Headers.STSIncludeSubdomains":                        "true",
		"ingress.HTTP.Middlewares.Middleware8.Headers.STSPreload":                                  "true",
		"ingress.HTTP.Middlewares.Middleware8.Headers.STSSeconds":                                  "42",
		"ingress.HTTP.Middlewares.Middleware9.IPAllowList.IPStrategy.Depth":                        "42",
		"ingress.HTTP.Middlewares.Middleware9.IPAllowList.IPStrategy.ExcludedIPs":                  "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware9.IPAllowList.IPStrategy.IPv6Subnet":                   "42",
		"ingress.HTTP.Middlewares.Middleware9.IPAllowList.RejectStatusCode":                        "0",
		"ingress.HTTP.Middlewares.Middleware9.IPAllowList.SourceRange":                             "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware10.InFlightReq.Amount":                                 "42",
		"ingress.HTTP.Middlewares.Middleware10.InFlightReq.SourceCriterion.IPStrategy.Depth":       "42",
		"ingress.HTTP.Middlewares.Middleware10.InFlightReq.SourceCriterion.IPStrategy.ExcludedIPs": "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware10.InFlightReq.SourceCriterion.IPStrategy.IPv6Subnet":  "42",
		"ingress.HTTP.Middlewares.Middleware10.InFlightReq.SourceCriterion.RequestHeaderName":      "foobar",
		"ingress.HTTP.Middlewares.Middleware10.InFlightReq.SourceCriterion.RequestHost":            "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.NotAfter":                    "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.NotBefore":                   "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Sans":                        "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.SerialNumber":                "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Subject.Country":             "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Subject.Province":            "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Subject.Locality":            "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Subject.Organization":        "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Subject.OrganizationalUnit":  "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Subject.CommonName":          "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Subject.SerialNumber":        "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Subject.DomainComponent":     "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Issuer.Country":              "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Issuer.Province":             "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Issuer.Locality":             "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Issuer.Organization":         "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Issuer.CommonName":           "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Issuer.SerialNumber":         "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.Info.Issuer.DomainComponent":      "true",
		"ingress.HTTP.Middlewares.Middleware11.PassTLSClientCert.PEM":                              "true",
		"ingress.HTTP.Middlewares.Middleware12.RateLimit.Average":                                  "42",
		"ingress.HTTP.Middlewares.Middleware12.RateLimit.Period":                                   "1000000000",
		"ingress.HTTP.Middlewares.Middleware12.RateLimit.Burst":                                    "42",
		"ingress.HTTP.Middlewares.Middleware12.RateLimit.SourceCriterion.RequestHeaderName":        "foobar",
		"ingress.HTTP.Middlewares.Middleware12.RateLimit.SourceCriterion.RequestHost":              "true",
		"ingress.HTTP.Middlewares.Middleware12.RateLimit.SourceCriterion.IPStrategy.Depth":         "42",
		"ingress.HTTP.Middlewares.Middleware12.RateLimit.SourceCriterion.IPStrategy.ExcludedIPs":   "foobar, foobar",
		"ingress.HTTP.Middlewares.Middleware12.RateLimit.SourceCriterion.IPStrategy.IPv6Subnet":    "42",
		"ingress.HTTP.Middlewares.Middleware13.RedirectRegex.Regex":                                "foobar",
		"ingress.HTTP.Middlewares.Middleware13.RedirectRegex.Replacement":                          "foobar",
		"ingress.HTTP.Middlewares.Middleware13.RedirectRegex.Permanent":                            "true",
		"ingress.HTTP.Middlewares.Middleware13b.RedirectScheme.Scheme":                             "https",
		"ingress.HTTP.Middlewares.Middleware13b.RedirectScheme.Port":                               "80",
		"ingress.HTTP.Middlewares.Middleware13b.RedirectScheme.Permanent":                          "true",
		"ingress.HTTP.Middlewares.Middleware14.ReplacePath.Path":                                   "foobar",
		"ingress.HTTP.Middlewares.Middleware15.ReplacePathRegex.Regex":                             "foobar",
		"ingress.HTTP.Middlewares.Middleware15.ReplacePathRegex.Replacement":                       "foobar",
		"ingress.HTTP.Middlewares.Middleware16.Retry.Attempts":                                     "42",
		"ingress.HTTP.Middlewares.Middleware16.Retry.InitialInterval":                              "1000000000",
		"ingress.HTTP.Middlewares.Middleware16.Retry.Timeout":                                      "1000000000",
		"ingress.HTTP.Middlewares.Middleware16.Retry.MaxRequestBodyBytes":                          "42",
		"ingress.HTTP.Middlewares.Middleware16.Retry.Status":                                       "foobar, foobar",
		"ingress.HTTP.Middlewares.Middleware16.Retry.DisableRetryOnNetworkError":                   "true",
		"ingress.HTTP.Middlewares.Middleware16.Retry.RetryNonIdempotentMethod":                     "true",
		"ingress.HTTP.Middlewares.Middleware17.StripPrefix.Prefixes":                               "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware17.StripPrefix.ForceSlash":                             "true",
		"ingress.HTTP.Middlewares.Middleware18.StripPrefixRegex.Regex":                             "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware19.Compress.Encodings":                                 "foobar, fiibar",
		"ingress.HTTP.Middlewares.Middleware19.Compress.MinResponseBodyBytes":                      "42",
		"ingress.HTTP.Middlewares.Middleware20.Plugin.tomato.aaa":                                  "foo1",
		"ingress.HTTP.Middlewares.Middleware20.Plugin.tomato.bbb":                                  "foo2",

		"ingress.HTTP.Routers.Router0.EntryPoints":              "foobar, fiibar",
		"ingress.HTTP.Routers.Router0.Middlewares":              "foobar, fiibar",
		"ingress.HTTP.Routers.Router0.Priority":                 "42",
		"ingress.HTTP.Routers.Router0.Rule":                     "foobar",
		"ingress.HTTP.Routers.Router0.Service":                  "foobar",
		"ingress.HTTP.Routers.Router0.TLS":                      "true",
		"ingress.HTTP.Routers.Router0.Observability.AccessLogs": "true",
		"ingress.HTTP.Routers.Router0.Observability.Tracing":    "true",
		"ingress.HTTP.Routers.Router0.Observability.Metrics":    "true",
		"ingress.HTTP.Routers.Router1.EntryPoints":              "foobar, fiibar",
		"ingress.HTTP.Routers.Router1.Middlewares":              "foobar, fiibar",
		"ingress.HTTP.Routers.Router1.Priority":                 "42",
		"ingress.HTTP.Routers.Router1.Rule":                     "foobar",
		"ingress.HTTP.Routers.Router1.Service":                  "foobar",
		"ingress.HTTP.Routers.Router1.Observability.AccessLogs": "true",
		"ingress.HTTP.Routers.Router1.Observability.Tracing":    "true",
		"ingress.HTTP.Routers.Router1.Observability.Metrics":    "true",

		"ingress.HTTP.Services.Service0.LoadBalancer.HealthCheck.Headers.name0":        "foobar",
		"ingress.HTTP.Services.Service0.LoadBalancer.HealthCheck.Headers.name1":        "foobar",
		"ingress.HTTP.Services.Service0.LoadBalancer.HealthCheck.Hostname":             "foobar",
		"ingress.HTTP.Services.Service0.LoadBalancer.HealthCheck.Interval":             "1000000000",
		"ingress.HTTP.Services.Service0.LoadBalancer.HealthCheck.UnhealthyInterval":    "1000000000",
		"ingress.HTTP.Services.Service0.LoadBalancer.HealthCheck.Path":                 "foobar",
		"ingress.HTTP.Services.Service0.LoadBalancer.HealthCheck.Method":               "foobar",
		"ingress.HTTP.Services.Service0.LoadBalancer.HealthCheck.Status":               "401",
		"ingress.HTTP.Services.Service0.LoadBalancer.HealthCheck.Port":                 "42",
		"ingress.HTTP.Services.Service0.LoadBalancer.HealthCheck.Scheme":               "foobar",
		"ingress.HTTP.Services.Service0.LoadBalancer.HealthCheck.Timeout":              "1000000000",
		"ingress.HTTP.Services.Service0.LoadBalancer.PassHostHeader":                   "true",
		"ingress.HTTP.Services.Service0.LoadBalancer.ResponseForwarding.FlushInterval": "1000000000",
		"ingress.HTTP.Services.Service0.LoadBalancer.Strategy":                         "foobar",
		"ingress.HTTP.Services.Service0.LoadBalancer.server.URL":                       "foobar",
		"ingress.HTTP.Services.Service0.LoadBalancer.server.PreservePath":              "true",
		"ingress.HTTP.Services.Service0.LoadBalancer.server.Port":                      "8080",
		"ingress.HTTP.Services.Service0.LoadBalancer.server.Scheme":                    "foobar",
		"ingress.HTTP.Services.Service0.LoadBalancer.Sticky.Cookie.Name":               "foobar",
		"ingress.HTTP.Services.Service0.LoadBalancer.Sticky.Cookie.HTTPOnly":           "true",
		"ingress.HTTP.Services.Service0.LoadBalancer.Sticky.Cookie.Secure":             "false",
		"ingress.HTTP.Services.Service0.LoadBalancer.Sticky.Cookie.MaxAge":             "0",
		"ingress.HTTP.Services.Service0.LoadBalancer.Sticky.Cookie.Path":               "/foobar",
		"ingress.HTTP.Services.Service0.LoadBalancer.Sticky.Cookie.Domain":             "foo.com",
		"ingress.HTTP.Services.Service0.LoadBalancer.ServersTransport":                 "foobar",
		"ingress.HTTP.Services.Service1.LoadBalancer.HealthCheck.Headers.name0":        "foobar",
		"ingress.HTTP.Services.Service1.LoadBalancer.HealthCheck.Headers.name1":        "foobar",
		"ingress.HTTP.Services.Service1.LoadBalancer.HealthCheck.Hostname":             "foobar",
		"ingress.HTTP.Services.Service1.LoadBalancer.HealthCheck.Interval":             "1000000000",
		"ingress.HTTP.Services.Service1.LoadBalancer.HealthCheck.UnhealthyInterval":    "1000000000",
		"ingress.HTTP.Services.Service1.LoadBalancer.HealthCheck.Path":                 "foobar",
		"ingress.HTTP.Services.Service1.LoadBalancer.HealthCheck.Method":               "foobar",
		"ingress.HTTP.Services.Service1.LoadBalancer.HealthCheck.Status":               "401",
		"ingress.HTTP.Services.Service1.LoadBalancer.HealthCheck.Port":                 "42",
		"ingress.HTTP.Services.Service1.LoadBalancer.HealthCheck.Scheme":               "foobar",
		"ingress.HTTP.Services.Service1.LoadBalancer.HealthCheck.Timeout":              "1000000000",
		"ingress.HTTP.Services.Service1.LoadBalancer.PassHostHeader":                   "true",
		"ingress.HTTP.Services.Service1.LoadBalancer.ResponseForwarding.FlushInterval": "1000000000",
		"ingress.HTTP.Services.Service1.LoadBalancer.Strategy":                         "foobar",
		"ingress.HTTP.Services.Service1.LoadBalancer.server.URL":                       "foobar",
		"ingress.HTTP.Services.Service1.LoadBalancer.server.PreservePath":              "true",
		"ingress.HTTP.Services.Service1.LoadBalancer.server.Port":                      "8080",
		"ingress.HTTP.Services.Service1.LoadBalancer.server.Scheme":                    "foobar",
		"ingress.HTTP.Services.Service1.LoadBalancer.ServersTransport":                 "foobar",

		"ingress.TCP.Middlewares.Middleware0.IPAllowList.SourceRange": "foobar, fiibar",
		"ingress.TCP.Middlewares.Middleware2.InFlightConn.Amount":     "42",
		"ingress.TCP.Routers.Router0.Rule":                            "foobar",
		"ingress.TCP.Routers.Router0.Priority":                        "42",
		"ingress.TCP.Routers.Router0.EntryPoints":                     "foobar, fiibar",
		"ingress.TCP.Routers.Router0.Service":                         "foobar",
		"ingress.TCP.Routers.Router0.TLS.Passthrough":                 "false",
		"ingress.TCP.Routers.Router0.TLS.Options":                     "foo",
		"ingress.TCP.Routers.Router1.Rule":                            "foobar",
		"ingress.TCP.Routers.Router1.Priority":                        "42",
		"ingress.TCP.Routers.Router1.EntryPoints":                     "foobar, fiibar",
		"ingress.TCP.Routers.Router1.Service":                         "foobar",
		"ingress.TCP.Routers.Router1.TLS.Passthrough":                 "false",
		"ingress.TCP.Routers.Router1.TLS.Options":                     "foo",
		"ingress.TCP.Services.Service0.LoadBalancer.server.Port":      "42",
		"ingress.TCP.Services.Service0.LoadBalancer.server.TLS":       "false",
		"ingress.TCP.Services.Service0.LoadBalancer.ServersTransport": "foo",
		"ingress.TCP.Services.Service0.LoadBalancer.TerminationDelay": "42",
		"ingress.TCP.Services.Service1.LoadBalancer.server.Port":      "42",
		"ingress.TCP.Services.Service1.LoadBalancer.server.TLS":       "false",
		"ingress.TCP.Services.Service1.LoadBalancer.ServersTransport": "foo",
		"ingress.TCP.Services.Service1.LoadBalancer.TerminationDelay": "42",

		"ingress.TLS.Stores.default.DefaultGeneratedCert.Resolver":    "foobar",
		"ingress.TLS.Stores.default.DefaultGeneratedCert.Domain.Main": "foobar",
		"ingress.TLS.Stores.default.DefaultGeneratedCert.Domain.SANs": "foobar, fiibar",

		"ingress.UDP.Routers.Router0.EntryPoints":                "foobar, fiibar",
		"ingress.UDP.Routers.Router0.Service":                    "foobar",
		"ingress.UDP.Routers.Router1.EntryPoints":                "foobar, fiibar",
		"ingress.UDP.Routers.Router1.Service":                    "foobar",
		"ingress.UDP.Services.Service0.LoadBalancer.server.Port": "42",
		"ingress.UDP.Services.Service1.LoadBalancer.server.Port": "42",
	}

	for key, val := range expected {
		if _, ok := labels[key]; !ok {
			fmt.Println("missing in labels:", key, val)
		}
	}

	for key, val := range labels {
		if _, ok := expected[key]; !ok {
			fmt.Println("missing in expected:", key, val)
		}
	}
	assert.Equal(t, expected, labels)
}

func intPtr(value int) *int {
	return &value
}
