// Package waf implements the web application firewall middleware.
//
// The engine is OWASP Coraza, which reads ModSecurity SecLang and is
// rule-compatible with the OWASP Core Rule Set. Rules reach it three ways,
// applied in this order so the later ones refine the earlier: the embedded
// Core Rule Set, files named by the configuration, then inline directives.
//
// A ruleset that matches interrupts the request and the configured status is
// returned. In DetectionOnly the same match is logged and the request
// proceeds, which is how a ruleset is measured against live traffic before it
// is allowed to refuse anything.
//
// The engine state is written after every rule, so this middleware decides
// whether the firewall refuses and no ruleset can decide it instead. That
// ordering is load-bearing rather than tidy: the Core Rule Set ships with the
// recommended engine configuration, whose seventh line is SecRuleEngine
// DetectionOnly, so a state written first is overridden by the rules it was
// meant to govern and the result loads all of OWASP CRS and refuses nothing.
//
// A configuration that loads no rule at all is refused where it is built. An
// engine with an empty ruleset answers every request successfully and inspects
// none of them, which reads as a firewall and is not one.
package waf

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	coreruleset "github.com/corazawaf/coraza-coreruleset/v4"
	"github.com/corazawaf/coraza/v3"
	corazahttp "github.com/corazawaf/coraza/v3/http"
	"github.com/corazawaf/coraza/v3/types"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
	"github.com/hanzoai/ingress/pkg/middlewares"
)

const typeName = "WAF"

// coreDirectives loads the recommended engine configuration, the Core Rule Set
// defaults, then the rules themselves. The three are one unit: the rules read
// variables the setup file defines, so loading them without it leaves the
// paranoia level and anomaly thresholds unset.
const coreDirectives = `Include @coraza.conf-recommended
Include @crs-setup.conf.example
Include @owasp_crs/*.conf`

type waf struct {
	handler http.Handler
	name    string
}

// New builds the firewall in front of next.
func New(ctx context.Context, next http.Handler, config dynamic.WAF, name string) (http.Handler, error) {
	logger := middlewares.GetLogger(ctx, name, typeName)

	cfg := coraza.NewWAFConfig().
		WithRootFS(coreruleset.FS).
		WithErrorCallback(func(rule types.MatchedRule) {
			logger.Warn().
				Int("rule", rule.Rule().ID()).
				Str("uri", rule.URI()).
				Str("transaction", rule.TransactionID()).
				Msg(rule.ErrorLog())
		})

	rules := 0
	if config.CoreRuleSet {
		cfg = cfg.WithDirectives(coreDirectives)
		rules++
	}
	for _, path := range config.DirectivesFiles {
		cfg = cfg.WithDirectivesFromFile(path)
		rules++
	}
	if strings.TrimSpace(config.Directives) != "" {
		cfg = cfg.WithDirectives(config.Directives)
		rules++
	}
	if rules == 0 {
		return nil, fmt.Errorf("%s: no rules — set coreRuleSet, directives or directivesFiles", typeName)
	}

	engine := "SecRuleEngine On"
	if config.DetectionOnly {
		engine = "SecRuleEngine DetectionOnly"
	}
	cfg = cfg.WithDirectives(engine)

	engineWAF, err := coraza.NewWAF(cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", typeName, err)
	}

	logger.Debug().
		Bool("coreRuleSet", config.CoreRuleSet).
		Bool("detectionOnly", config.DetectionOnly).
		Msg("Creating middleware")

	return &waf{handler: corazahttp.WrapHandler(engineWAF, next), name: name}, nil
}

func (w *waf) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	w.handler.ServeHTTP(rw, req)
}

func (w *waf) GetTracingInformation() (string, string) {
	return w.name, typeName
}
