package kv

import (
	"errors"
	"testing"
	"time"

	"github.com/kvtools/valkeyrie/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ptypes "github.com/traefik/paerser/types"
	"github.com/hanzoai/ingress/v3/pkg/config/dynamic"
	"github.com/hanzoai/ingress/v3/pkg/tls"
	"github.com/hanzoai/ingress/v3/pkg/types"
)

func pointer[T any](v T) *T { return &v }

func Test_buildConfiguration(t *testing.T) {
	provider := newProviderMock(mapToPairs(map[string]string{
		"ingress/http/routers/Router0/entryPoints/0":                                                 "foobar",
		"ingress/http/routers/Router0/entryPoints/1":                                                 "foobar",
		"ingress/http/routers/Router0/middlewares/0":                                                 "foobar",
		"ingress/http/routers/Router0/middlewares/1":                                                 "foobar",
		"ingress/http/routers/Router0/service":                                                       "foobar",
		"ingress/http/routers/Router0/rule":                                                          "foobar",
		"ingress/http/routers/Router0/priority":                                                      "42",
		"ingress/http/routers/Router0/tls":                                                           "",
		"ingress/http/routers/Router1/rule":                                                          "foobar",
		"ingress/http/routers/Router1/priority":                                                      "42",
		"ingress/http/routers/Router1/tls/domains/0/main":                                            "foobar",
		"ingress/http/routers/Router1/tls/domains/0/sans/0":                                          "foobar",
		"ingress/http/routers/Router1/tls/domains/0/sans/1":                                          "foobar",
		"ingress/http/routers/Router1/tls/domains/1/main":                                            "foobar",
		"ingress/http/routers/Router1/tls/domains/1/sans/0":                                          "foobar",
		"ingress/http/routers/Router1/tls/domains/1/sans/1":                                          "foobar",
		"ingress/http/routers/Router1/tls/options":                                                   "foobar",
		"ingress/http/routers/Router1/tls/certResolver":                                              "foobar",
		"ingress/http/routers/Router1/entryPoints/0":                                                 "foobar",
		"ingress/http/routers/Router1/entryPoints/1":                                                 "foobar",
		"ingress/http/routers/Router1/middlewares/0":                                                 "foobar",
		"ingress/http/routers/Router1/middlewares/1":                                                 "foobar",
		"ingress/http/routers/Router1/service":                                                       "foobar",
		"ingress/http/services/Service01/loadBalancer/healthCheck/path":                              "foobar",
		"ingress/http/services/Service01/loadBalancer/healthCheck/port":                              "42",
		"ingress/http/services/Service01/loadBalancer/healthCheck/interval":                          "1s",
		"ingress/http/services/Service01/loadBalancer/healthCheck/unhealthyinterval":                 "1s",
		"ingress/http/services/Service01/loadBalancer/healthCheck/timeout":                           "1s",
		"ingress/http/services/Service01/loadBalancer/healthCheck/hostname":                          "foobar",
		"ingress/http/services/Service01/loadBalancer/healthCheck/headers/name0":                     "foobar",
		"ingress/http/services/Service01/loadBalancer/healthCheck/headers/name1":                     "foobar",
		"ingress/http/services/Service01/loadBalancer/healthCheck/scheme":                            "foobar",
		"ingress/http/services/Service01/loadBalancer/healthCheck/mode":                              "foobar",
		"ingress/http/services/Service01/loadBalancer/healthCheck/followredirects":                   "true",
		"ingress/http/services/Service01/loadBalancer/responseForwarding/flushInterval":              "1s",
		"ingress/http/services/Service01/loadBalancer/passHostHeader":                                "true",
		"ingress/http/services/Service01/loadBalancer/sticky/cookie/name":                            "foobar",
		"ingress/http/services/Service01/loadBalancer/sticky/cookie/secure":                          "true",
		"ingress/http/services/Service01/loadBalancer/sticky/cookie/httpOnly":                        "true",
		"ingress/http/services/Service01/loadBalancer/sticky/cookie/path":                            "foobar",
		"ingress/http/services/Service01/loadBalancer/strategy":                                      "foobar",
		"ingress/http/services/Service01/loadBalancer/servers/0/url":                                 "foobar",
		"ingress/http/services/Service01/loadBalancer/servers/1/url":                                 "foobar",
		"ingress/http/services/Service02/mirroring/service":                                          "foobar",
		"ingress/http/services/Service02/mirroring/mirrorBody":                                       "true",
		"ingress/http/services/Service02/mirroring/maxBodySize":                                      "42",
		"ingress/http/services/Service02/mirroring/mirrors/0/name":                                   "foobar",
		"ingress/http/services/Service02/mirroring/mirrors/0/percent":                                "42",
		"ingress/http/services/Service02/mirroring/mirrors/1/name":                                   "foobar",
		"ingress/http/services/Service02/mirroring/mirrors/1/percent":                                "42",
		"ingress/http/services/Service03/weighted/sticky/cookie/name":                                "foobar",
		"ingress/http/services/Service03/weighted/sticky/cookie/secure":                              "true",
		"ingress/http/services/Service03/weighted/sticky/cookie/httpOnly":                            "true",
		"ingress/http/services/Service03/weighted/sticky/cookie/path":                                "foobar",
		"ingress/http/services/Service03/weighted/services/0/name":                                   "foobar",
		"ingress/http/services/Service03/weighted/services/0/weight":                                 "42",
		"ingress/http/services/Service03/weighted/services/1/name":                                   "foobar",
		"ingress/http/services/Service03/weighted/services/1/weight":                                 "42",
		"ingress/http/services/Service04/failover/service":                                           "foobar",
		"ingress/http/services/Service04/failover/fallback":                                          "foobar",
		"ingress/http/middlewares/Middleware08/forwardAuth/authResponseHeaders/0":                    "foobar",
		"ingress/http/middlewares/Middleware08/forwardAuth/authResponseHeaders/1":                    "foobar",
		"ingress/http/middlewares/Middleware08/forwardAuth/authRequestHeaders/0":                     "foobar",
		"ingress/http/middlewares/Middleware08/forwardAuth/authRequestHeaders/1":                     "foobar",
		"ingress/http/middlewares/Middleware08/forwardAuth/tls/key":                                  "foobar",
		"ingress/http/middlewares/Middleware08/forwardAuth/tls/insecureSkipVerify":                   "true",
		"ingress/http/middlewares/Middleware08/forwardAuth/tls/ca":                                   "foobar",
		"ingress/http/middlewares/Middleware08/forwardAuth/tls/caOptional":                           "true",
		"ingress/http/middlewares/Middleware08/forwardAuth/tls/cert":                                 "foobar",
		"ingress/http/middlewares/Middleware08/forwardAuth/address":                                  "foobar",
		"ingress/http/middlewares/Middleware08/forwardAuth/trustForwardHeader":                       "true",
		"ingress/http/middlewares/Middleware08/forwardAuth/forwardBody":                              "true",
		"ingress/http/middlewares/Middleware08/forwardAuth/maxBodySize":                              "42",
		"ingress/http/middlewares/Middleware08/forwardAuth/preserveLocationHeader":                   "true",
		"ingress/http/middlewares/Middleware08/forwardAuth/preserveRequestMethod":                    "true",
		"ingress/http/middlewares/Middleware08/forwardAuth/maxResponseBodySize":                      "42",
		"ingress/http/middlewares/Middleware15/redirectScheme/scheme":                                "foobar",
		"ingress/http/middlewares/Middleware15/redirectScheme/port":                                  "foobar",
		"ingress/http/middlewares/Middleware15/redirectScheme/permanent":                             "true",
		"ingress/http/middlewares/Middleware17/replacePathRegex/regex":                               "foobar",
		"ingress/http/middlewares/Middleware17/replacePathRegex/replacement":                         "foobar",
		"ingress/http/middlewares/Middleware14/redirectRegex/regex":                                  "foobar",
		"ingress/http/middlewares/Middleware14/redirectRegex/replacement":                            "foobar",
		"ingress/http/middlewares/Middleware14/redirectRegex/permanent":                              "true",
		"ingress/http/middlewares/Middleware16/replacePath/path":                                     "foobar",
		"ingress/http/middlewares/Middleware06/digestAuth/removeHeader":                              "true",
		"ingress/http/middlewares/Middleware06/digestAuth/realm":                                     "foobar",
		"ingress/http/middlewares/Middleware06/digestAuth/headerField":                               "foobar",
		"ingress/http/middlewares/Middleware06/digestAuth/users/0":                                   "foobar",
		"ingress/http/middlewares/Middleware06/digestAuth/users/1":                                   "foobar",
		"ingress/http/middlewares/Middleware06/digestAuth/usersFile":                                 "foobar",
		"ingress/http/middlewares/Middleware09/headers/accessControlAllowHeaders/0":                  "foobar",
		"ingress/http/middlewares/Middleware09/headers/accessControlAllowHeaders/1":                  "foobar",
		"ingress/http/middlewares/Middleware09/headers/accessControlAllowOriginList/0":               "foobar",
		"ingress/http/middlewares/Middleware09/headers/accessControlAllowOriginList/1":               "foobar",
		"ingress/http/middlewares/Middleware09/headers/accessControlAllowOriginListRegex/0":          "foobar",
		"ingress/http/middlewares/Middleware09/headers/accessControlAllowOriginListRegex/1":          "foobar",
		"ingress/http/middlewares/Middleware09/headers/contentTypeNosniff":                           "true",
		"ingress/http/middlewares/Middleware09/headers/accessControlAllowCredentials":                "true",
		"ingress/http/middlewares/Middleware09/headers/featurePolicy":                                "foobar",
		"ingress/http/middlewares/Middleware09/headers/permissionsPolicy":                            "foobar",
		"ingress/http/middlewares/Middleware09/headers/forceSTSHeader":                               "true",
		"ingress/http/middlewares/Middleware09/headers/sslRedirect":                                  "true",
		"ingress/http/middlewares/Middleware09/headers/sslHost":                                      "foobar",
		"ingress/http/middlewares/Middleware09/headers/sslForceHost":                                 "true",
		"ingress/http/middlewares/Middleware09/headers/sslProxyHeaders/name1":                        "foobar",
		"ingress/http/middlewares/Middleware09/headers/sslProxyHeaders/name0":                        "foobar",
		"ingress/http/middlewares/Middleware09/headers/allowedHosts/0":                               "foobar",
		"ingress/http/middlewares/Middleware09/headers/allowedHosts/1":                               "foobar",
		"ingress/http/middlewares/Middleware09/headers/stsPreload":                                   "true",
		"ingress/http/middlewares/Middleware09/headers/frameDeny":                                    "true",
		"ingress/http/middlewares/Middleware09/headers/isDevelopment":                                "true",
		"ingress/http/middlewares/Middleware09/headers/customResponseHeaders/name1":                  "foobar",
		"ingress/http/middlewares/Middleware09/headers/customResponseHeaders/name0":                  "foobar",
		"ingress/http/middlewares/Middleware09/headers/accessControlAllowMethods/0":                  "foobar",
		"ingress/http/middlewares/Middleware09/headers/accessControlAllowMethods/1":                  "foobar",
		"ingress/http/middlewares/Middleware09/headers/stsSeconds":                                   "42",
		"ingress/http/middlewares/Middleware09/headers/stsIncludeSubdomains":                         "true",
		"ingress/http/middlewares/Middleware09/headers/customFrameOptionsValue":                      "foobar",
		"ingress/http/middlewares/Middleware09/headers/accessControlMaxAge":                          "42",
		"ingress/http/middlewares/Middleware09/headers/addVaryHeader":                                "true",
		"ingress/http/middlewares/Middleware09/headers/hostsProxyHeaders/0":                          "foobar",
		"ingress/http/middlewares/Middleware09/headers/hostsProxyHeaders/1":                          "foobar",
		"ingress/http/middlewares/Middleware09/headers/sslTemporaryRedirect":                         "true",
		"ingress/http/middlewares/Middleware09/headers/customBrowserXSSValue":                        "foobar",
		"ingress/http/middlewares/Middleware09/headers/referrerPolicy":                               "foobar",
		"ingress/http/middlewares/Middleware09/headers/accessControlExposeHeaders/0":                 "foobar",
		"ingress/http/middlewares/Middleware09/headers/accessControlExposeHeaders/1":                 "foobar",
		"ingress/http/middlewares/Middleware09/headers/contentSecurityPolicy":                        "foobar",
		"ingress/http/middlewares/Middleware09/headers/contentSecurityPolicyReportOnly":              "foobar",
		"ingress/http/middlewares/Middleware09/headers/publicKey":                                    "foobar",
		"ingress/http/middlewares/Middleware09/headers/customRequestHeaders/name0":                   "foobar",
		"ingress/http/middlewares/Middleware09/headers/customRequestHeaders/name1":                   "foobar",
		"ingress/http/middlewares/Middleware09/headers/browserXssFilter":                             "true",
		"ingress/http/middlewares/Middleware10/ipAllowList/sourceRange/0":                            "foobar",
		"ingress/http/middlewares/Middleware10/ipAllowList/sourceRange/1":                            "foobar",
		"ingress/http/middlewares/Middleware10/ipAllowList/ipStrategy/excludedIPs/0":                 "foobar",
		"ingress/http/middlewares/Middleware10/ipAllowList/ipStrategy/excludedIPs/1":                 "foobar",
		"ingress/http/middlewares/Middleware10/ipAllowList/ipStrategy/depth":                         "42",
		"ingress/http/middlewares/Middleware11/inFlightReq/amount":                                   "42",
		"ingress/http/middlewares/Middleware11/inFlightReq/sourceCriterion/requestHost":              "true",
		"ingress/http/middlewares/Middleware11/inFlightReq/sourceCriterion/ipStrategy/depth":         "42",
		"ingress/http/middlewares/Middleware11/inFlightReq/sourceCriterion/ipStrategy/excludedIPs/0": "foobar",
		"ingress/http/middlewares/Middleware11/inFlightReq/sourceCriterion/ipStrategy/excludedIPs/1": "foobar",
		"ingress/http/middlewares/Middleware11/inFlightReq/sourceCriterion/requestHeaderName":        "foobar",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/pem":                                "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/notAfter":                      "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/notBefore":                     "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/sans":                          "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/subject/country":               "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/subject/province":              "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/subject/locality":              "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/subject/organization":          "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/subject/organizationalunit":    "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/subject/commonName":            "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/subject/serialNumber":          "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/subject/domainComponent":       "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/issuer/country":                "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/issuer/province":               "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/issuer/locality":               "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/issuer/organization":           "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/issuer/commonName":             "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/issuer/serialNumber":           "true",
		"ingress/http/middlewares/Middleware12/passTLSClientCert/info/issuer/domainComponent":        "true",
		"ingress/http/middlewares/Middleware00/addPrefix/prefix":                                     "foobar",
		"ingress/http/middlewares/Middleware03/chain/middlewares/0":                                  "foobar",
		"ingress/http/middlewares/Middleware03/chain/middlewares/1":                                  "foobar",
		"ingress/http/middlewares/Middleware04/circuitBreaker/expression":                            "foobar",
		"ingress/http/middlewares/Middleware04/circuitBreaker/checkPeriod":                           "1s",
		"ingress/http/middlewares/Middleware04/circuitBreaker/fallbackDuration":                      "1s",
		"ingress/http/middlewares/Middleware04/circuitBreaker/recoveryDuration":                      "1s",
		"ingress/http/middlewares/Middleware04/circuitBreaker/responseCode":                          "404",
		"ingress/http/middlewares/Middleware07/errors/status/0":                                      "foobar",
		"ingress/http/middlewares/Middleware07/errors/status/1":                                      "foobar",
		"ingress/http/middlewares/Middleware07/errors/service":                                       "foobar",
		"ingress/http/middlewares/Middleware07/errors/query":                                         "foobar",
		"ingress/http/middlewares/Middleware13/rateLimit/average":                                    "42",
		"ingress/http/middlewares/Middleware13/rateLimit/period":                                     "1s",
		"ingress/http/middlewares/Middleware13/rateLimit/burst":                                      "42",
		"ingress/http/middlewares/Middleware13/rateLimit/sourceCriterion/requestHeaderName":          "foobar",
		"ingress/http/middlewares/Middleware13/rateLimit/sourceCriterion/requestHost":                "true",
		"ingress/http/middlewares/Middleware13/rateLimit/sourceCriterion/ipStrategy/depth":           "42",
		"ingress/http/middlewares/Middleware13/rateLimit/sourceCriterion/ipStrategy/excludedIPs/0":   "foobar",
		"ingress/http/middlewares/Middleware13/rateLimit/sourceCriterion/ipStrategy/excludedIPs/1":   "foobar",
		"ingress/http/middlewares/Middleware20/stripPrefixRegex/regex/0":                             "foobar",
		"ingress/http/middlewares/Middleware20/stripPrefixRegex/regex/1":                             "foobar",
		"ingress/http/middlewares/Middleware01/basicAuth/users/0":                                    "foobar",
		"ingress/http/middlewares/Middleware01/basicAuth/users/1":                                    "foobar",
		"ingress/http/middlewares/Middleware01/basicAuth/usersFile":                                  "foobar",
		"ingress/http/middlewares/Middleware01/basicAuth/realm":                                      "foobar",
		"ingress/http/middlewares/Middleware01/basicAuth/removeHeader":                               "true",
		"ingress/http/middlewares/Middleware01/basicAuth/headerField":                                "foobar",
		"ingress/http/middlewares/Middleware02/buffering/maxResponseBodyBytes":                       "42",
		"ingress/http/middlewares/Middleware02/buffering/memResponseBodyBytes":                       "42",
		"ingress/http/middlewares/Middleware02/buffering/retryExpression":                            "foobar",
		"ingress/http/middlewares/Middleware02/buffering/maxRequestBodyBytes":                        "42",
		"ingress/http/middlewares/Middleware02/buffering/memRequestBodyBytes":                        "42",
		"ingress/http/middlewares/Middleware05/compress/encodings":                                   "foobar, foobar",
		"ingress/http/middlewares/Middleware05/compress/minResponseBodyBytes":                        "42",
		"ingress/http/middlewares/Middleware18/retry/attempts":                                       "42",
		"ingress/http/middlewares/Middleware18/retry/timeout":                                        "1s",
		"ingress/http/middlewares/Middleware18/retry/initialInterval":                                "1s",
		"ingress/http/middlewares/Middleware18/retry/maxRequestBodyBytes":                            "42",
		"ingress/http/middlewares/Middleware18/retry/status":                                         "400,500-599",
		"ingress/http/middlewares/Middleware18/retry/disableRetryOnNetworkError":                     "true",
		"ingress/http/middlewares/Middleware18/retry/retryNonIdempotentMethod":                       "true",
		"ingress/http/middlewares/Middleware19/stripPrefix/prefixes/0":                               "foobar",
		"ingress/http/middlewares/Middleware19/stripPrefix/prefixes/1":                               "foobar",
		"ingress/http/middlewares/Middleware19/stripPrefix/forceSlash":                               "true",
		"ingress/tcp/routers/TCPRouter0/entryPoints/0":                                               "foobar",
		"ingress/tcp/routers/TCPRouter0/entryPoints/1":                                               "foobar",
		"ingress/tcp/routers/TCPRouter0/service":                                                     "foobar",
		"ingress/tcp/routers/TCPRouter0/rule":                                                        "foobar",
		"ingress/tcp/routers/TCPRouter0/tls/options":                                                 "foobar",
		"ingress/tcp/routers/TCPRouter0/tls/certResolver":                                            "foobar",
		"ingress/tcp/routers/TCPRouter0/tls/domains/0/main":                                          "foobar",
		"ingress/tcp/routers/TCPRouter0/tls/domains/0/sans/0":                                        "foobar",
		"ingress/tcp/routers/TCPRouter0/tls/domains/0/sans/1":                                        "foobar",
		"ingress/tcp/routers/TCPRouter0/tls/domains/1/main":                                          "foobar",
		"ingress/tcp/routers/TCPRouter0/tls/domains/1/sans/0":                                        "foobar",
		"ingress/tcp/routers/TCPRouter0/tls/domains/1/sans/1":                                        "foobar",
		"ingress/tcp/routers/TCPRouter0/tls/passthrough":                                             "true",
		"ingress/tcp/routers/TCPRouter1/entryPoints/0":                                               "foobar",
		"ingress/tcp/routers/TCPRouter1/entryPoints/1":                                               "foobar",
		"ingress/tcp/routers/TCPRouter1/service":                                                     "foobar",
		"ingress/tcp/routers/TCPRouter1/rule":                                                        "foobar",
		"ingress/tcp/routers/TCPRouter1/tls/domains/0/main":                                          "foobar",
		"ingress/tcp/routers/TCPRouter1/tls/domains/0/sans/0":                                        "foobar",
		"ingress/tcp/routers/TCPRouter1/tls/domains/0/sans/1":                                        "foobar",
		"ingress/tcp/routers/TCPRouter1/tls/domains/1/main":                                          "foobar",
		"ingress/tcp/routers/TCPRouter1/tls/domains/1/sans/0":                                        "foobar",
		"ingress/tcp/routers/TCPRouter1/tls/domains/1/sans/1":                                        "foobar",
		"ingress/tcp/routers/TCPRouter1/tls/passthrough":                                             "true",
		"ingress/tcp/routers/TCPRouter1/tls/options":                                                 "foobar",
		"ingress/tcp/routers/TCPRouter1/tls/certResolver":                                            "foobar",
		"ingress/tcp/services/TCPService01/loadBalancer/terminationDelay":                            "42",
		"ingress/tcp/services/TCPService01/loadBalancer/servers/0/address":                           "foobar",
		"ingress/tcp/services/TCPService01/loadBalancer/servers/1/address":                           "foobar",
		"ingress/tcp/services/TCPService02/weighted/services/0/name":                                 "foobar",
		"ingress/tcp/services/TCPService02/weighted/services/0/weight":                               "42",
		"ingress/tcp/services/TCPService02/weighted/services/1/name":                                 "foobar",
		"ingress/tcp/services/TCPService02/weighted/services/1/weight":                               "43",
		"ingress/udp/routers/UDPRouter0/entrypoints/0":                                               "foobar",
		"ingress/udp/routers/UDPRouter0/entrypoints/1":                                               "foobar",
		"ingress/udp/routers/UDPRouter0/service":                                                     "foobar",
		"ingress/udp/routers/UDPRouter1/entrypoints/0":                                               "foobar",
		"ingress/udp/routers/UDPRouter1/entrypoints/1":                                               "foobar",
		"ingress/udp/routers/UDPRouter1/service":                                                     "foobar",
		"ingress/udp/services/UDPService01/loadBalancer/servers/0/address":                           "foobar",
		"ingress/udp/services/UDPService01/loadBalancer/servers/1/address":                           "foobar",
		"ingress/udp/services/UDPService02/loadBalancer/servers/0/address":                           "foobar",
		"ingress/udp/services/UDPService02/loadBalancer/servers/1/address":                           "foobar",
		"ingress/tls/options/Options0/minVersion":                                                    "foobar",
		"ingress/tls/options/Options0/maxVersion":                                                    "foobar",
		"ingress/tls/options/Options0/cipherSuites/0":                                                "foobar",
		"ingress/tls/options/Options0/cipherSuites/1":                                                "foobar",
		"ingress/tls/options/Options0/sniStrict":                                                     "true",
		"ingress/tls/options/Options0/curvePreferences/0":                                            "foobar",
		"ingress/tls/options/Options0/curvePreferences/1":                                            "foobar",
		"ingress/tls/options/Options0/clientAuth/caFiles/0":                                          "foobar",
		"ingress/tls/options/Options0/clientAuth/caFiles/1":                                          "foobar",
		"ingress/tls/options/Options0/clientAuth/clientAuthType":                                     "foobar",
		"ingress/tls/options/Options1/sniStrict":                                                     "true",
		"ingress/tls/options/Options1/curvePreferences/0":                                            "foobar",
		"ingress/tls/options/Options1/curvePreferences/1":                                            "foobar",
		"ingress/tls/options/Options1/clientAuth/caFiles/0":                                          "foobar",
		"ingress/tls/options/Options1/clientAuth/caFiles/1":                                          "foobar",
		"ingress/tls/options/Options1/clientAuth/clientAuthType":                                     "foobar",
		"ingress/tls/options/Options1/minVersion":                                                    "foobar",
		"ingress/tls/options/Options1/maxVersion":                                                    "foobar",
		"ingress/tls/options/Options1/cipherSuites/0":                                                "foobar",
		"ingress/tls/options/Options1/cipherSuites/1":                                                "foobar",
		"ingress/tls/stores/Store0/defaultCertificate/certFile":                                      "foobar",
		"ingress/tls/stores/Store0/defaultCertificate/keyFile":                                       "foobar",
		"ingress/tls/stores/Store1/defaultCertificate/certFile":                                      "foobar",
		"ingress/tls/stores/Store1/defaultCertificate/keyFile":                                       "foobar",
		"ingress/tls/certificates/0/certFile":                                                        "foobar",
		"ingress/tls/certificates/0/keyFile":                                                         "foobar",
		"ingress/tls/certificates/0/stores/0":                                                        "foobar",
		"ingress/tls/certificates/0/stores/1":                                                        "foobar",
		"ingress/tls/certificates/1/certFile":                                                        "foobar",
		"ingress/tls/certificates/1/keyFile":                                                         "foobar",
		"ingress/tls/certificates/1/stores/0":                                                        "foobar",
		"ingress/tls/certificates/1/stores/1":                                                        "foobar",
	}))

	cfg, err := provider.buildConfiguration(t.Context())
	require.NoError(t, err)

	expected := &dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{
			Routers: map[string]*dynamic.Router{
				"Router1": {
					EntryPoints: []string{
						"foobar",
						"foobar",
					},
					Middlewares: []string{
						"foobar",
						"foobar",
					},
					Service:  "foobar",
					Rule:     "foobar",
					Priority: 42,
					TLS: &dynamic.RouterTLSConfig{
						Options:      "foobar",
						CertResolver: "foobar",
						Domains: []types.Domain{
							{
								Main: "foobar",
								SANs: []string{
									"foobar",
									"foobar",
								},
							},
							{
								Main: "foobar",
								SANs: []string{
									"foobar",
									"foobar",
								},
							},
						},
					},
				},
				"Router0": {
					EntryPoints: []string{
						"foobar",
						"foobar",
					},
					Middlewares: []string{
						"foobar",
						"foobar",
					},
					Service:  "foobar",
					Rule:     "foobar",
					Priority: 42,
					TLS:      &dynamic.RouterTLSConfig{},
				},
			},
			Middlewares: map[string]*dynamic.Middleware{
				"Middleware10": {
					IPAllowList: &dynamic.IPAllowList{
						SourceRange: []string{
							"foobar",
							"foobar",
						},
						IPStrategy: &dynamic.IPStrategy{
							Depth: 42,
							ExcludedIPs: []string{
								"foobar",
								"foobar",
							},
						},
					},
				},
				"Middleware13": {
					RateLimit: &dynamic.RateLimit{
						Average: 42,
						Burst:   42,
						Period:  ptypes.Duration(time.Second),
						SourceCriterion: &dynamic.SourceCriterion{
							IPStrategy: &dynamic.IPStrategy{
								Depth: 42,
								ExcludedIPs: []string{
									"foobar",
									"foobar",
								},
							},
							RequestHeaderName: "foobar",
							RequestHost:       true,
						},
					},
				},
				"Middleware19": {
					StripPrefix: &dynamic.StripPrefix{
						Prefixes: []string{
							"foobar",
							"foobar",
						},
						ForceSlash: pointer(true),
					},
				},
				"Middleware00": {
					AddPrefix: &dynamic.AddPrefix{
						Prefix: "foobar",
					},
				},
				"Middleware02": {
					Buffering: &dynamic.Buffering{
						MaxRequestBodyBytes:  42,
						MemRequestBodyBytes:  42,
						MaxResponseBodyBytes: 42,
						MemResponseBodyBytes: 42,
						RetryExpression:      "foobar",
					},
				},
				"Middleware04": {
					CircuitBreaker: &dynamic.CircuitBreaker{
						Expression:       "foobar",
						CheckPeriod:      ptypes.Duration(time.Second),
						FallbackDuration: ptypes.Duration(time.Second),
						RecoveryDuration: ptypes.Duration(time.Second),
						ResponseCode:     404,
					},
				},
				"Middleware05": {
					Compress: &dynamic.Compress{
						MinResponseBodyBytes: 42,
						Encodings: []string{
							"foobar",
							"foobar",
						},
					},
				},
				"Middleware08": {
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
							"foobar",
						},
						AuthRequestHeaders: []string{
							"foobar",
							"foobar",
						},
						MaxResponseBodySize:    pointer[int64](42),
						ForwardBody:            true,
						MaxBodySize:            pointer(int64(42)),
						PreserveLocationHeader: true,
						PreserveRequestMethod:  true,
					},
				},
				"Middleware06": {
					DigestAuth: &dynamic.DigestAuth{
						Users: dynamic.Users{
							"foobar",
							"foobar",
						},
						UsersFile:    "foobar",
						RemoveHeader: true,
						Realm:        "foobar",
						HeaderField:  "foobar",
					},
				},
				"Middleware18": {
					Retry: &dynamic.Retry{
						Attempts:                   42,
						Timeout:                    ptypes.Duration(time.Second),
						InitialInterval:            ptypes.Duration(time.Second),
						MaxRequestBodyBytes:        pointer[int64](42),
						Status:                     []string{"400", "500-599"},
						DisableRetryOnNetworkError: true,
						RetryNonIdempotentMethod:   true,
					},
				},
				"Middleware16": {
					ReplacePath: &dynamic.ReplacePath{
						Path: "foobar",
					},
				},
				"Middleware20": {
					StripPrefixRegex: &dynamic.StripPrefixRegex{
						Regex: []string{
							"foobar",
							"foobar",
						},
					},
				},
				"Middleware03": {
					Chain: &dynamic.Chain{
						Middlewares: []string{
							"foobar",
							"foobar",
						},
					},
				},
				"Middleware11": {
					InFlightReq: &dynamic.InFlightReq{
						Amount: 42,
						SourceCriterion: &dynamic.SourceCriterion{
							IPStrategy: &dynamic.IPStrategy{
								Depth: 42,
								ExcludedIPs: []string{
									"foobar",
									"foobar",
								},
							},
							RequestHeaderName: "foobar",
							RequestHost:       true,
						},
					},
				},
				"Middleware12": {
					PassTLSClientCert: &dynamic.PassTLSClientCert{
						PEM: true,
						Info: &dynamic.TLSClientCertificateInfo{
							NotAfter:  true,
							NotBefore: true,
							Sans:      true,
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
						},
					},
				},
				"Middleware14": {
					RedirectRegex: &dynamic.RedirectRegex{
						Regex:       "foobar",
						Replacement: "foobar",
						Permanent:   true,
					},
				},
				"Middleware15": {
					RedirectScheme: &dynamic.RedirectScheme{
						Scheme:    "foobar",
						Port:      "foobar",
						Permanent: true,
					},
				},
				"Middleware01": {
					BasicAuth: &dynamic.BasicAuth{
						Users: dynamic.Users{
							"foobar",
							"foobar",
						},
						UsersFile:    "foobar",
						Realm:        "foobar",
						RemoveHeader: true,
						HeaderField:  "foobar",
					},
				},
				"Middleware07": {
					Errors: &dynamic.ErrorPage{
						Status: []string{
							"foobar",
							"foobar",
						},
						Service: "foobar",
						Query:   "foobar",
					},
				},
				"Middleware09": {
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
							"foobar",
							"foobar",
						},
						AccessControlAllowMethods: []string{
							"foobar",
							"foobar",
						},
						AccessControlAllowOriginList: []string{
							"foobar",
							"foobar",
						},
						AccessControlAllowOriginListRegex: []string{
							"foobar",
							"foobar",
						},
						AccessControlExposeHeaders: []string{
							"foobar",
							"foobar",
						},
						AccessControlMaxAge: 42,
						AddVaryHeader:       true,
						AllowedHosts: []string{
							"foobar",
							"foobar",
						},
						HostsProxyHeaders: []string{
							"foobar",
							"foobar",
						},
						SSLRedirect:          pointer(true),
						SSLTemporaryRedirect: pointer(true),
						SSLHost:              pointer("foobar"),
						SSLProxyHeaders: map[string]string{
							"name1": "foobar",
							"name0": "foobar",
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
				"Middleware17": {
					ReplacePathRegex: &dynamic.ReplacePathRegex{
						Regex:       "foobar",
						Replacement: "foobar",
					},
				},
			},
			Services: map[string]*dynamic.Service{
				"Service01": {
					LoadBalancer: &dynamic.ServersLoadBalancer{
						Strategy: "foobar",
						Sticky: &dynamic.Sticky{
							Cookie: &dynamic.Cookie{
								Name:     "foobar",
								Secure:   true,
								HTTPOnly: true,
								Path:     func(v string) *string { return &v }("foobar"),
							},
						},
						Servers: []dynamic.Server{
							{
								URL: "foobar",
							},
							{
								URL: "foobar",
							},
						},
						HealthCheck: &dynamic.ServerHealthCheck{
							Scheme:            "foobar",
							Mode:              "foobar",
							Path:              "foobar",
							Port:              42,
							Interval:          ptypes.Duration(time.Second),
							UnhealthyInterval: pointer(ptypes.Duration(time.Second)),
							Timeout:           ptypes.Duration(time.Second),
							Hostname:          "foobar",
							FollowRedirects:   pointer(true),
							Headers: map[string]string{
								"name0": "foobar",
								"name1": "foobar",
							},
						},
						PassHostHeader: pointer(true),
						ResponseForwarding: &dynamic.ResponseForwarding{
							FlushInterval: ptypes.Duration(time.Second),
						},
					},
				},
				"Service02": {
					Mirroring: &dynamic.Mirroring{
						Service:     "foobar",
						MirrorBody:  pointer(true),
						MaxBodySize: pointer[int64](42),
						Mirrors: []dynamic.MirrorService{
							{
								Name:    "foobar",
								Percent: 42,
							},
							{
								Name:    "foobar",
								Percent: 42,
							},
						},
					},
				},
				"Service03": {
					Weighted: &dynamic.WeightedRoundRobin{
						Services: []dynamic.WRRService{
							{
								Name:   "foobar",
								Weight: pointer(42),
							},
							{
								Name:   "foobar",
								Weight: pointer(42),
							},
						},
						Sticky: &dynamic.Sticky{
							Cookie: &dynamic.Cookie{
								Name:     "foobar",
								Secure:   true,
								HTTPOnly: true,
								Path:     func(v string) *string { return &v }("foobar"),
							},
						},
					},
				},
				"Service04": {
					Failover: &dynamic.Failover{
						Service:  "foobar",
						Fallback: "foobar",
					},
				},
			},
		},
		TCP: &dynamic.TCPConfiguration{
			Routers: map[string]*dynamic.TCPRouter{
				"TCPRouter0": {
					EntryPoints: []string{
						"foobar",
						"foobar",
					},
					Service: "foobar",
					Rule:    "foobar",
					TLS: &dynamic.RouterTCPTLSConfig{
						Passthrough:  true,
						Options:      "foobar",
						CertResolver: "foobar",
						Domains: []types.Domain{
							{
								Main: "foobar",
								SANs: []string{
									"foobar",
									"foobar",
								},
							},
							{
								Main: "foobar",
								SANs: []string{
									"foobar",
									"foobar",
								},
							},
						},
					},
				},
				"TCPRouter1": {
					EntryPoints: []string{
						"foobar",
						"foobar",
					},
					Service: "foobar",
					Rule:    "foobar",
					TLS: &dynamic.RouterTCPTLSConfig{
						Passthrough:  true,
						Options:      "foobar",
						CertResolver: "foobar",
						Domains: []types.Domain{
							{
								Main: "foobar",
								SANs: []string{
									"foobar",
									"foobar",
								},
							},
							{
								Main: "foobar",
								SANs: []string{
									"foobar",
									"foobar",
								},
							},
						},
					},
				},
			},
			Services: map[string]*dynamic.TCPService{
				"TCPService01": {
					LoadBalancer: &dynamic.TCPServersLoadBalancer{
						TerminationDelay: pointer(42),
						Servers: []dynamic.TCPServer{
							{Address: "foobar"},
							{Address: "foobar"},
						},
					},
				},
				"TCPService02": {
					Weighted: &dynamic.TCPWeightedRoundRobin{
						Services: []dynamic.TCPWRRService{
							{
								Name:   "foobar",
								Weight: pointer(42),
							},
							{
								Name:   "foobar",
								Weight: pointer(43),
							},
						},
					},
				},
			},
		},
		UDP: &dynamic.UDPConfiguration{
			Routers: map[string]*dynamic.UDPRouter{
				"UDPRouter0": {
					EntryPoints: []string{"foobar", "foobar"},
					Service:     "foobar",
				},
				"UDPRouter1": {
					EntryPoints: []string{"foobar", "foobar"},
					Service:     "foobar",
				},
			},
			Services: map[string]*dynamic.UDPService{
				"UDPService01": {
					LoadBalancer: &dynamic.UDPServersLoadBalancer{
						Servers: []dynamic.UDPServer{
							{Address: "foobar"},
							{Address: "foobar"},
						},
					},
				},
				"UDPService02": {
					LoadBalancer: &dynamic.UDPServersLoadBalancer{
						Servers: []dynamic.UDPServer{
							{Address: "foobar"},
							{Address: "foobar"},
						},
					},
				},
			},
		},
		TLS: &dynamic.TLSConfiguration{
			Certificates: []*tls.CertAndStores{
				{
					Certificate: tls.Certificate{
						CertFile: types.FileOrContent("foobar"),
						KeyFile:  types.FileOrContent("foobar"),
					},
					Stores: []string{
						"foobar",
						"foobar",
					},
				},
				{
					Certificate: tls.Certificate{
						CertFile: types.FileOrContent("foobar"),
						KeyFile:  types.FileOrContent("foobar"),
					},
					Stores: []string{
						"foobar",
						"foobar",
					},
				},
			},
			Options: map[string]tls.Options{
				"Options0": {
					MinVersion: "foobar",
					MaxVersion: "foobar",
					CipherSuites: []string{
						"foobar",
						"foobar",
					},
					CurvePreferences: []string{
						"foobar",
						"foobar",
					},
					ClientAuth: tls.ClientAuth{
						CAFiles: []types.FileOrContent{
							types.FileOrContent("foobar"),
							types.FileOrContent("foobar"),
						},
						ClientAuthType: "foobar",
					},
					SniStrict: true,
					ALPNProtocols: []string{
						"h2",
						"http/1.1",
						"acme-tls/1",
					},
				},
				"Options1": {
					MinVersion: "foobar",
					MaxVersion: "foobar",
					CipherSuites: []string{
						"foobar",
						"foobar",
					},
					CurvePreferences: []string{
						"foobar",
						"foobar",
					},
					ClientAuth: tls.ClientAuth{
						CAFiles: []types.FileOrContent{
							types.FileOrContent("foobar"),
							types.FileOrContent("foobar"),
						},
						ClientAuthType: "foobar",
					},
					SniStrict: true,
					ALPNProtocols: []string{
						"h2",
						"http/1.1",
						"acme-tls/1",
					},
				},
			},
			Stores: map[string]tls.Store{
				"Store0": {
					DefaultCertificate: &tls.Certificate{
						CertFile: types.FileOrContent("foobar"),
						KeyFile:  types.FileOrContent("foobar"),
					},
				},
				"Store1": {
					DefaultCertificate: &tls.Certificate{
						CertFile: types.FileOrContent("foobar"),
						KeyFile:  types.FileOrContent("foobar"),
					},
				},
			},
		},
	}

	assert.Equal(t, expected, cfg)
}

func Test_buildConfiguration_KV_error(t *testing.T) {
	provider := &Provider{
		RootKey: "ingress",
		kvClient: &Mock{
			Error: KvError{
				List: errors.New("OOPS"),
			},
			KVPairs: mapToPairs(map[string]string{
				"ingress/foo": "bar",
			}),
		},
	}

	cfg, err := provider.buildConfiguration(t.Context())
	require.Error(t, err)
	assert.Nil(t, cfg)
}

func TestKvWatchTree(t *testing.T) {
	returnedChans := make(chan chan []*store.KVPair)
	provider := Provider{
		kvClient: &Mock{
			WatchTreeMethod: func() <-chan []*store.KVPair {
				c := make(chan []*store.KVPair, 10)
				returnedChans <- c
				return c
			},
		},
	}

	configChan := make(chan dynamic.Message)
	go func() {
		err := provider.watchKv(t.Context(), configChan)
		require.NoError(t, err)
	}()

	select {
	case c1 := <-returnedChans:
		c1 <- []*store.KVPair{}
		<-configChan
		close(c1) // WatchTree chans can close due to error
	case <-time.After(1 * time.Second):
		t.Fatalf("Failed to create a new WatchTree chan")
	}

	select {
	case c2 := <-returnedChans:
		c2 <- []*store.KVPair{}
		<-configChan
	case <-time.After(1 * time.Second):
		t.Fatalf("Failed to create a new WatchTree chan")
	}

	select {
	case <-configChan:
		t.Fatalf("configChan should be empty")
	default:
	}
}

func mapToPairs(in map[string]string) []*store.KVPair {
	var out []*store.KVPair
	for k, v := range in {
		out = append(out, &store.KVPair{Key: k, Value: []byte(v)})
	}
	return out
}
