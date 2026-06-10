// Package geoblock implements the GeoIP-based country block + header
// injection middleware.
//
// One source of truth for "what country is this request from" across the
// entire Hanzo ingress stack — used by Hanzo, Lux, Zoo, and every other
// brand behind the ingress. Two behaviours, both optional and composable:
//
//  1. Country-code header injection. Resolves the client IP to ISO 3166-1
//     alpha-2 and sets `X-Geo-Country: US` (default header name) on the
//     upstream request. Downstream apps read this once and never need
//     their own MaxMind code path.
//
//  2. Country-level access control. Optional explicit block list (for
//     deny-by-country, e.g. OFAC comprehensive sanctions) or allow list
//     (for whitelist-mode where the service only serves a fixed set of
//     jurisdictions). Mutually exclusive — allow takes precedence when
//     both are configured.
//
// Resolution strategy (first hit wins):
//
//  1. Trusted upstream proxy header — `CF-IPCountry` or
//     `X-Forwarded-Country`. Honoured when set so this middleware
//     composes correctly behind Cloudflare / Fastly / etc.
//  2. MaxMind GeoLite2-Country.mmdb lookup of the client IP.
//
// When the resolver returns no country (private IP, lookup failure,
// missing DB) the middleware fails OPEN: header is not set, no block
// fires, request proceeds. That matches the existing IPAllowList
// posture — sanctions are an explicit gate, not a side effect of
// infrastructure flakiness.
package geoblock

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/oschwald/maxminddb-golang"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
	"github.com/hanzoai/ingress/pkg/ip"
	"github.com/hanzoai/ingress/pkg/middlewares"
	"github.com/hanzoai/ingress/pkg/middlewares/observability"
)

const (
	typeName              = "GeoBlock"
	defaultHeader         = "X-Geo-Country"
	defaultRejectStatus   = http.StatusUnavailableForLegalReasons // 451
	upstreamCountryHeader = "X-Forwarded-Country"
	cloudflareIPCountry   = "CF-IPCountry"
)

// resolver returns the ISO 3166-1 alpha-2 country code for the given IP,
// or empty string when the IP cannot be resolved. Implementations must
// be safe for concurrent use across goroutines.
type resolver interface {
	Lookup(ip net.IP) string
	Close() error
}

// noopResolver returns empty for every lookup — used when no DB is
// configured AND no upstream-trust header is present. The middleware
// then degrades to header-pass-through.
type noopResolver struct{}

func (noopResolver) Lookup(net.IP) string { return "" }
func (noopResolver) Close() error         { return nil }

// mmdbResolver wraps an open MaxMind GeoLite2-Country DB.
type mmdbResolver struct {
	db *maxminddb.Reader
}

func newMMDBResolver(path string) (*mmdbResolver, error) {
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("geoblock: open mmdb %s: %w", path, err)
	}
	return &mmdbResolver{db: db}, nil
}

func (m *mmdbResolver) Lookup(addr net.IP) string {
	var rec struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}
	if err := m.db.Lookup(addr, &rec); err != nil {
		return ""
	}
	return strings.ToUpper(rec.Country.ISOCode)
}

func (m *mmdbResolver) Close() error { return m.db.Close() }

// geoBlock is the assembled middleware.
type geoBlock struct {
	next             http.Handler
	strategy         ip.Strategy
	resolver         resolver
	headerName       string
	allowSet         map[string]struct{}
	blockSet         map[string]struct{}
	allowMode        bool
	rejectStatusCode int
	name             string
}

// New constructs a GeoBlock middleware.
func New(ctx context.Context, next http.Handler, config dynamic.GeoBlock, name string) (http.Handler, error) {
	logger := middlewares.GetLogger(ctx, name, typeName)
	logger.Debug().Msg("Creating middleware")

	strategy, err := (&dynamic.IPStrategy{}).Get()
	if err != nil {
		return nil, fmt.Errorf("geoblock: default IP strategy: %w", err)
	}
	if config.IPStrategy != nil {
		strategy, err = config.IPStrategy.Get()
		if err != nil {
			return nil, fmt.Errorf("geoblock: IP strategy: %w", err)
		}
	}

	headerName := strings.TrimSpace(config.HeaderName)
	if headerName == "" {
		headerName = defaultHeader
	}

	rejectStatus := config.RejectStatusCode
	if rejectStatus == 0 {
		rejectStatus = defaultRejectStatus
	} else if http.StatusText(rejectStatus) == "" {
		return nil, fmt.Errorf("geoblock: invalid HTTP status code %d", rejectStatus)
	}

	var res resolver = noopResolver{}
	if path := strings.TrimSpace(config.DatabasePath); path != "" {
		r, err := newMMDBResolver(path)
		if err != nil {
			// Fail-open with a loud warning rather than refusing to mount —
			// a missing/corrupt MMDB shouldn't take the whole ingress down.
			logger.Warn().Err(err).Msgf("geoblock: failed to open MMDB at %s, falling back to upstream-trust only", path)
		} else {
			res = r
		}
	}

	allowSet := make(map[string]struct{}, len(config.AllowCountries))
	for _, c := range config.AllowCountries {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c != "" {
			allowSet[c] = struct{}{}
		}
	}
	blockSet := make(map[string]struct{}, len(config.BlockCountries))
	for _, c := range config.BlockCountries {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c != "" {
			blockSet[c] = struct{}{}
		}
	}

	allowMode := len(allowSet) > 0
	if allowMode && len(blockSet) > 0 {
		logger.Warn().Msg("geoblock: AllowCountries set; BlockCountries ignored (allow takes precedence)")
	}

	logger.Debug().Msgf("geoblock ready: header=%s allowMode=%v allow=%v block=%v",
		headerName, allowMode, config.AllowCountries, config.BlockCountries)

	return &geoBlock{
		next:             next,
		strategy:         strategy,
		resolver:         res,
		headerName:       headerName,
		allowSet:         allowSet,
		blockSet:         blockSet,
		allowMode:        allowMode,
		rejectStatusCode: rejectStatus,
		name:             name,
	}, nil
}

func (g *geoBlock) GetTracingInformation() (string, string) {
	return g.name, typeName
}

// resolveCountry returns the ISO alpha-2 country code or empty.
func (g *geoBlock) resolveCountry(req *http.Request) string {
	// 1. Trusted upstream header (Cloudflare, our own X-Forwarded-Country).
	for _, h := range []string{cloudflareIPCountry, upstreamCountryHeader} {
		if v := strings.ToUpper(strings.TrimSpace(req.Header.Get(h))); v != "" && v != "XX" && v != "T1" {
			return v
		}
	}
	// 2. MaxMind lookup of resolved client IP.
	addrStr := g.strategy.GetIP(req)
	if addrStr == "" {
		return ""
	}
	addr := net.ParseIP(addrStr)
	if addr == nil {
		return ""
	}
	return g.resolver.Lookup(addr)
}

func (g *geoBlock) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	logger := middlewares.GetLogger(req.Context(), g.name, typeName)

	country := g.resolveCountry(req)

	if country == "" {
		// Fail-open: no country resolved (private IP, missing DB, etc.).
		// Header isn't set, block isn't enforced — pass through.
		g.next.ServeHTTP(rw, req)
		return
	}

	if g.allowMode {
		if _, ok := g.allowSet[country]; !ok {
			logger.Info().Msgf("geoblock: rejecting %s — not in allow list", country)
			observability.SetStatusErrorf(req.Context(), "geo-block: country %s not allowed", country)
			req.Header.Set(g.headerName+"-Reason", "country_not_allowed:"+country)
			rejectWith(rw, g.rejectStatusCode, country)
			return
		}
	} else if _, ok := g.blockSet[country]; ok {
		logger.Info().Msgf("geoblock: rejecting %s — in block list", country)
		observability.SetStatusErrorf(req.Context(), "geo-block: country %s blocked", country)
		req.Header.Set(g.headerName+"-Reason", "country_blocked:"+country)
		rejectWith(rw, g.rejectStatusCode, country)
		return
	}

	// Inject the resolved country into the upstream request so downstream
	// apps don't need their own MaxMind code path.
	req.Header.Set(g.headerName, country)
	g.next.ServeHTTP(rw, req)
}

func rejectWith(rw http.ResponseWriter, status int, country string) {
	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	rw.Header().Set("X-Block-Reason", "geo:"+country)
	rw.WriteHeader(status)
	_, _ = rw.Write([]byte("Service is not available in your region.\n"))
}

// ErrNoDatabase is returned by callers that explicitly require an MMDB
// to be present (e.g. boot-time validation).
var ErrNoDatabase = errors.New("geoblock: no MaxMind database configured")
