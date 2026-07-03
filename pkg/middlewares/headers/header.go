package headers

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
	"github.com/hanzoai/ingress/pkg/middlewares"
	"github.com/vulcand/oxy/v2/forward"
)

// Header is a middleware that helps setup a few basic security features.
// A single headerOptions struct can be provided to configure which features should be enabled,
// and the ability to override a few of the default values.
type Header struct {
	next               http.Handler
	hasCustomHeaders   bool
	hasCorsHeaders     bool
	headers            *dynamic.Headers
	allowOriginRegexes []*regexp.Regexp
}

// NewHeader constructs a new header instance from supplied frontend header struct.
func NewHeader(next http.Handler, cfg dynamic.Headers) (*Header, error) {
	hasCustomHeaders := cfg.HasCustomHeadersDefined()
	hasCorsHeaders := cfg.HasCorsHeadersDefined()

	regexes := make([]*regexp.Regexp, len(cfg.AccessControlAllowOriginListRegex))
	for i, str := range cfg.AccessControlAllowOriginListRegex {
		reg, err := regexp.Compile(anchorOriginRegex(str))
		if err != nil {
			return nil, fmt.Errorf("error occurred during origin parsing: %w", err)
		}
		regexes[i] = reg
	}

	return &Header{
		next:               next,
		headers:            &cfg,
		hasCustomHeaders:   hasCustomHeaders,
		hasCorsHeaders:     hasCorsHeaders,
		allowOriginRegexes: regexes,
	}, nil
}

func (s *Header) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// Handle Cors headers and preflight if configured.
	if isPreflight := s.processCorsHeaders(rw, req); isPreflight {
		rw.Header().Set("Content-Length", "0")
		rw.WriteHeader(http.StatusOK)
		return
	}

	if s.hasCustomHeaders {
		s.modifyCustomRequestHeaders(req)
	}

	// If there is a next, call it.
	if s.next != nil {
		s.next.ServeHTTP(middlewares.NewResponseModifier(rw, req, s.PostRequestModifyResponseHeaders), req)
	}
}

// PostRequestModifyResponseHeaders set or delete response headers.
// This method is called AFTER the response is generated from the backend
// and can merge/override headers from the backend response.
func (s *Header) PostRequestModifyResponseHeaders(res *http.Response) error {
	// Loop through Custom response headers
	for header, value := range s.headers.CustomResponseHeaders {
		if value == "" {
			res.Header.Del(header)
		} else {
			res.Header.Set(header, value)
		}
	}

	// Access-Control-Allow-Credentials MUST only be emitted alongside a matched
	// Access-Control-Allow-Origin. Emitting it unconditionally lets a
	// credentialed response pair with a reflected/foreign origin set by another
	// layer (backend or CDN) — the near-account-takeover class. Gate the
	// credentials header on the same allow decision as the origin.
	originAllowed := false
	if res != nil && res.Request != nil {
		originHeader := res.Request.Header.Get("Origin")
		allowed, match := s.isOriginAllowed(originHeader)
		originAllowed = allowed

		if allowed {
			res.Header.Set("Access-Control-Allow-Origin", match)
		}
	}

	if originAllowed && s.headers.AccessControlAllowCredentials {
		res.Header.Set("Access-Control-Allow-Credentials", "true")
	}

	if len(s.headers.AccessControlExposeHeaders) > 0 {
		exposeHeaders := strings.Join(s.headers.AccessControlExposeHeaders, ",")
		res.Header.Set("Access-Control-Expose-Headers", exposeHeaders)
	}

	if !s.headers.AddVaryHeader {
		return nil
	}

	varyHeader := res.Header.Get("Vary")
	if varyHeader == "Origin" {
		return nil
	}

	if varyHeader != "" {
		varyHeader += ","
	}
	varyHeader += "Origin"

	res.Header.Set("Vary", varyHeader)
	return nil
}

// modifyCustomRequestHeaders sets or deletes custom request headers.
func (s *Header) modifyCustomRequestHeaders(req *http.Request) {
	// Loop through Custom request headers
	for header, value := range s.headers.CustomRequestHeaders {
		switch {
		// Handling https://github.com/golang/go/commit/ecdbffd4ec68b509998792f120868fec319de59b.
		case value == "" && header == forward.XForwardedFor:
			req.Header[header] = nil

		case value == "":
			req.Header.Del(header)

		case strings.EqualFold(header, "Host"):
			req.Host = value

		default:
			req.Header.Set(header, value)
		}
	}
}

// processCorsHeaders processes the incoming request,
// and returns if it is a preflight request.
// If not a preflight, it handles the preRequestModifyCorsResponseHeaders.
func (s *Header) processCorsHeaders(rw http.ResponseWriter, req *http.Request) bool {
	if !s.hasCorsHeaders {
		return false
	}

	reqAcMethod := req.Header.Get("Access-Control-Request-Method")
	originHeader := req.Header.Get("Origin")

	if reqAcMethod != "" && originHeader != "" && req.Method == http.MethodOptions {
		// If the request is an OPTIONS request with an Access-Control-Request-Method header,
		// and Origin headers, then it is a CORS preflight request,
		// and we need to build a custom response: https://www.w3.org/TR/cors/#preflight-request
		allowed, match := s.isOriginAllowed(originHeader)
		if allowed {
			rw.Header().Set("Access-Control-Allow-Origin", match)
		}

		// Only advertise credentialed CORS to an allowlisted origin. Emitting
		// Access-Control-Allow-Credentials for an unmatched origin is the
		// near-account-takeover class (it pairs with a reflected origin).
		if allowed && s.headers.AccessControlAllowCredentials {
			rw.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		allowHeaders := strings.Join(s.headers.AccessControlAllowHeaders, ",")
		if allowHeaders != "" {
			rw.Header().Set("Access-Control-Allow-Headers", allowHeaders)
		}

		allowMethods := strings.Join(s.headers.AccessControlAllowMethods, ",")
		if allowMethods != "" {
			rw.Header().Set("Access-Control-Allow-Methods", allowMethods)
		}

		rw.Header().Set("Access-Control-Max-Age", strconv.Itoa(int(s.headers.AccessControlMaxAge)))
		return true
	}

	return false
}

func (s *Header) isOriginAllowed(origin string) (bool, string) {
	for _, item := range s.headers.AccessControlAllowOriginList {
		if item == "*" || item == origin {
			return true, item
		}
	}

	for _, regex := range s.allowOriginRegexes {
		if regex.MatchString(origin) {
			return true, origin
		}
	}

	return false, ""
}

// anchorOriginRegex forces a configured origin pattern to match the WHOLE
// origin, never a substring. regexp.MatchString is unanchored, so an operator
// pattern like `https://([a-z0-9-]+\.)?hanzo\.ai` matches the attacker origin
// `https://hanzo.ai.evil.com` (found as a prefix) and would reflect it — the
// credentialed-CORS hole. We wrap every pattern in `^(?:…)$` so the match is
// full-string. The `(?:…)` group keeps top-level alternation intact (`a|b`
// would otherwise anchor as `^a|b$`). Any anchors the operator already added
// are stripped first so the result is never a doubly-anchored, never-matching
// `^(?:^…$)$`. Anchoring in code is defense in depth: even a mis-authored,
// unanchored CRD/middleware config can no longer re-open the hole.
func anchorOriginRegex(pattern string) string {
	p := strings.TrimPrefix(pattern, "^")
	p = strings.TrimSuffix(p, "$")
	return "^(?:" + p + ")$"
}
