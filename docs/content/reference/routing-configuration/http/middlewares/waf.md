---
title: "Hanzo Ingress WAF Documentation"
description: "The WAF middleware in Hanzo Ingress inspects every request against OWASP Core Rule Set and ModSecurity SecLang rules. Read the technical documentation."
---

The WAF middleware is a web application firewall in the request path. The
engine is [OWASP Coraza](https://coraza.io), which reads ModSecurity SecLang
and is rule-compatible with the [OWASP Core Rule Set](https://coreruleset.org)
v4. Both ship inside the ingress binary; nothing is fetched at start.

A request that matches a blocking rule is refused with the status the rule
names — 403 for the Core Rule Set — and never reaches the service. In
`detectionOnly` the same match is logged and the request proceeds, which is
how a ruleset is measured against live traffic before it is allowed to refuse.

## Configuration Examples

```yaml tab="Structured (YAML)"
# Refuse what the Core Rule Set refuses
http:
  middlewares:
    waf:
      waf:
        coreRuleSet: true
```

```toml tab="Structured (TOML)"
# Refuse what the Core Rule Set refuses
[http.middlewares]
  [http.middlewares.waf.waf]
    coreRuleSet = true
```

```yaml tab="Labels"
# Refuse what the Core Rule Set refuses
labels:
  - "ingress.http.middlewares.waf.waf.coreRuleSet=true"
```

```yaml tab="Kubernetes"
# Refuse what the Core Rule Set refuses
apiVersion: hanzo.ai/v1alpha1
kind: Middleware
metadata:
  name: waf
spec:
  waf:
    coreRuleSet: true
```

## Configuration Options

| Field | Description | Default | Required |
|:------|:------------|:--------|:---------|
| `coreRuleSet` | Load the embedded OWASP Core Rule Set v4, with the recommended engine configuration and the setup file the rules read. | false | No |
| `directivesFiles` | SecLang files to load, in the order given, after the Core Rule Set. | | No |
| `directives` | SecLang written inline, applied after every file. | | No |
| `detectionOnly` | Log every match and refuse nothing. | false | No |

At least one of `coreRuleSet`, `directivesFiles` or `directives` is required.
A configuration that loads no rule is refused at start: an engine with an
empty ruleset answers every request successfully and inspects none of them.

### Order

Rules are applied Core Rule Set → files → inline, so a later source refines
an earlier one. That is where a site-specific exclusion goes:

```yaml tab="Kubernetes"
apiVersion: hanzo.ai/v1alpha1
kind: Middleware
metadata:
  name: waf
spec:
  waf:
    coreRuleSet: true
    directives: |
      SecRuleRemoveById 942100
      SecRule REQUEST_URI "@beginsWith /healthz" "id:10001,phase:1,pass,nolog,ctl:ruleEngine=Off"
```

The engine state is written after every rule, so `detectionOnly` is the one
place the mode is decided. A `SecRuleEngine` line inside a ruleset — the Core
Rule Set's own recommended configuration carries `SecRuleEngine DetectionOnly`
— cannot disarm the middleware that loaded it.

### Paranoia and thresholds

The Core Rule Set scores each request and refuses it past a threshold. The
defaults are paranoia level 1 and an inbound threshold of 5, set by the
embedded setup file. Raise either with inline directives:

```yaml
directives: |
  SecAction "id:900000,phase:1,pass,t:none,nolog,setvar:tx.blocking_paranoia_level=2"
  SecAction "id:900110,phase:1,pass,t:none,nolog,setvar:tx.inbound_anomaly_score_threshold=10"
```

A higher paranoia level refuses more and refuses more legitimate traffic. Run
it in `detectionOnly` first and read the log.

## Logging

Every match is written at `WARN` with the rule id, the transaction id and the
request URI, whether or not it refused the request.
