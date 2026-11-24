package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
	"github.com/hanzoai/ingress/pkg/middlewares"
)

const (
	typeNameIAM = "IAMAuth"

	defaultIAMIssuer       = "https://hanzo.id"
	defaultIAMCookieName   = "hanzo_iam_session"
	defaultIAMLoginPath    = "/auth/login"
	defaultIAMCallbackPath = "/callback"
	defaultIAMLogoutPath   = "/auth/logout"
	defaultIAMScopes       = "openid profile email"
)

// untrustedIdentityHeaders are ALWAYS stripped from incoming client requests
// before auth runs so no client can forge identity or SuperAdmin headers.
var untrustedIdentityHeaders = []string{
	"X-Org-Id",
	"X-Project-Id",
	"X-User-Id",
	"X-User-Name",
	"X-User-Email",
	"X-User-Owner",
	"X-User-IsAdmin",
	"X-User-IsOrgAdmin",
	"X-Acted-By",
	"X-Billing-Account-Id",
	"X-Hanzo-Sub",
	"X-Hanzo-User",
	"X-Hanzo-Org",
	"X-Hanzo-Owner",
	"X-Hanzo-Email",
	"X-Hanzo-SuperAdmin",
}

type iamAuth struct {
	next            http.Handler
	name            string
	cfg             dynamic.IAMAuth
	browserRedirect bool
	client          *http.Client
	jwks            *iamJWKSCache
}

type iamPrincipal struct {
	Sub        string
	Org        string
	Owner      string
	Username   string
	Email      string
	Project    string
	SuperAdmin bool // true iff Owner == "admin" (never derived from org isAdmin)
	OrgAdmin   bool
	Token      string
}

// NewIAM creates a Hanzo IAM OIDC PKCE + JWKS edge authentication middleware.
func NewIAM(ctx context.Context, next http.Handler, config dynamic.IAMAuth, name string) (http.Handler, error) {
	logger := middlewares.GetLogger(ctx, name, typeNameIAM)
	logger.Debug().Msg("Creating middleware")

	if config.Issuer == "" {
		config.Issuer = defaultIAMIssuer
	}
	config.Issuer = strings.TrimRight(config.Issuer, "/")
	if config.JWKSURL == "" {
		config.JWKSURL = config.Issuer + "/v1/iam/.well-known/jwks"
	}
	if config.AuthorizeURL == "" {
		config.AuthorizeURL = config.Issuer + "/v1/iam/oauth/authorize"
	}
	if config.TokenURL == "" {
		config.TokenURL = config.Issuer + "/v1/iam/oauth/token"
	}
	if config.Scopes == "" {
		config.Scopes = defaultIAMScopes
	}
	if config.CookieName == "" {
		config.CookieName = defaultIAMCookieName
	}
	if config.LoginPath == "" {
		config.LoginPath = defaultIAMLoginPath
	}
	if config.CallbackPath == "" {
		config.CallbackPath = defaultIAMCallbackPath
	}
	if config.LogoutPath == "" {
		config.LogoutPath = defaultIAMLogoutPath
	}

	redir := true
	if config.BrowserRedirect != nil {
		redir = *config.BrowserRedirect
	}

	client := &http.Client{Timeout: 10 * time.Second}
	return &iamAuth{
		next:            next,
		name:            name,
		cfg:             config,
		browserRedirect: redir,
		client:          client,
		jwks:            newIAMJWKSCache(config.JWKSURL, client, 10*time.Minute),
	}, nil
}

func (a *iamAuth) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// Always strip spoofable incoming identity headers before any branch runs.
	for _, h := range untrustedIdentityHeaders {
		req.Header.Del(h)
	}

	switch req.URL.Path {
	case a.cfg.LoginPath:
		a.handleLogin(rw, req)
		return
	case a.cfg.CallbackPath:
		a.handleCallback(rw, req)
		return
	case a.cfg.LogoutPath:
		a.handleLogout(rw, req)
		return
	}

	token := iamBearerToken(req.Header.Get("Authorization"))
	if token == "" {
		if ck, err := req.Cookie(a.cfg.CookieName); err == nil {
			token = ck.Value
		}
	}

	if token != "" {
		if principal, err := a.verifyToken(req.Context(), token); err == nil {
			injectIAMHeaders(req, principal)
			a.next.ServeHTTP(rw, req)
			return
		}
	}

	if a.isSkipped(req.URL.Path) || a.cfg.Optional {
		a.next.ServeHTTP(rw, req)
		return
	}

	if a.browserRedirect && iamWantsHTML(req) && a.cfg.ClientID != "" {
		nextURL := req.URL.RequestURI()
		target := a.cfg.LoginPath + "?next=" + url.QueryEscape(iamSanitizeLocalRedirect(nextURL))
		http.Redirect(rw, req, target, http.StatusFound)
		return
	}

	rw.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s"`, a.cfg.Issuer))
	writeIAMJSONError(rw, http.StatusUnauthorized, "authentication required")
}

func (a *iamAuth) handleLogin(rw http.ResponseWriter, req *http.Request) {
	if a.cfg.ClientID == "" {
		writeIAMJSONError(rw, http.StatusInternalServerError, "IAM clientID is not configured")
		return
	}
	state, err := iamRandomToken(24)
	if err != nil {
		writeIAMJSONError(rw, http.StatusInternalServerError, "failed to generate state")
		return
	}
	verifier, err := iamRandomToken(32)
	if err != nil {
		writeIAMJSONError(rw, http.StatusInternalServerError, "failed to generate pkce verifier")
		return
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	next := iamSanitizeLocalRedirect(req.URL.Query().Get("next"))
	if next == "/" {
		if r := req.URL.Query().Get("redirect"); r != "" {
			next = iamSanitizeLocalRedirect(r)
		}
	}

	txVal := base64.RawURLEncoding.EncodeToString([]byte(state + "|" + verifier + "|" + next))
	http.SetCookie(rw, &http.Cookie{
		Name:     a.cfg.CookieName + "_tx",
		Value:    txVal,
		Path:     "/",
		Domain:   a.cfg.CookieDomain,
		MaxAge:   600,
		HttpOnly: true,
		Secure:   iamIsSecure(req),
		SameSite: http.SameSiteLaxMode,
	})

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", a.cfg.ClientID)
	q.Set("redirect_uri", a.redirectURI(req))
	q.Set("scope", a.cfg.Scopes)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	http.Redirect(rw, req, a.cfg.AuthorizeURL+"?"+q.Encode(), http.StatusFound)
}

func (a *iamAuth) handleCallback(rw http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	if errCode := q.Get("error"); errCode != "" {
		desc := q.Get("error_description")
		if desc == "" {
			desc = errCode
		}
		writeIAMJSONError(rw, http.StatusUnauthorized, "IAM login failed: "+desc)
		return
	}
	code := q.Get("code")
	state := q.Get("state")
	if code == "" || state == "" {
		writeIAMJSONError(rw, http.StatusBadRequest, "missing code or state")
		return
	}

	txCookie, err := req.Cookie(a.cfg.CookieName + "_tx")
	// Always clear transaction cookie after single use.
	http.SetCookie(rw, &http.Cookie{
		Name:     a.cfg.CookieName + "_tx",
		Value:    "",
		Path:     "/",
		Domain:   a.cfg.CookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   iamIsSecure(req),
		SameSite: http.SameSiteLaxMode,
	})
	if err != nil {
		writeIAMJSONError(rw, http.StatusUnauthorized, "missing login transaction")
		return
	}

	rawTx, err := base64.RawURLEncoding.DecodeString(txCookie.Value)
	if err != nil {
		writeIAMJSONError(rw, http.StatusUnauthorized, "invalid login transaction")
		return
	}
	parts := strings.SplitN(string(rawTx), "|", 3)
	if len(parts) != 3 || !hmac.Equal([]byte(parts[0]), []byte(state)) {
		writeIAMJSONError(rw, http.StatusUnauthorized, "invalid OAuth state")
		return
	}
	verifier := parts[1]
	next := iamSanitizeLocalRedirect(parts[2])

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", a.cfg.ClientID)
	form.Set("redirect_uri", a.redirectURI(req))
	form.Set("code_verifier", verifier)
	if a.cfg.ClientSecret != "" {
		form.Set("client_secret", a.cfg.ClientSecret)
	}

	tokReq, err := http.NewRequestWithContext(req.Context(), http.MethodPost, a.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		writeIAMJSONError(rw, http.StatusInternalServerError, "failed to build token request")
		return
	}
	tokReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokReq.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(tokReq)
	if err != nil {
		writeIAMJSONError(rw, http.StatusBadGateway, "IAM token exchange failed")
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeIAMJSONError(rw, http.StatusUnauthorized, "IAM token exchange rejected")
		return
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		writeIAMJSONError(rw, http.StatusUnauthorized, "invalid IAM token response")
		return
	}
	jwtStr := tok.AccessToken
	if jwtStr == "" {
		jwtStr = tok.IDToken
	}
	if jwtStr == "" {
		writeIAMJSONError(rw, http.StatusUnauthorized, "IAM returned empty token")
		return
	}
	if _, err := a.verifyToken(req.Context(), jwtStr); err != nil {
		writeIAMJSONError(rw, http.StatusUnauthorized, "IAM token verification failed")
		return
	}

	maxAge := tok.ExpiresIn
	if maxAge <= 0 {
		maxAge = 3600
	}
	http.SetCookie(rw, &http.Cookie{
		Name:     a.cfg.CookieName,
		Value:    jwtStr,
		Path:     "/",
		Domain:   a.cfg.CookieDomain,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   iamIsSecure(req),
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(rw, req, next, http.StatusFound)
}

func (a *iamAuth) handleLogout(rw http.ResponseWriter, req *http.Request) {
	for _, name := range []string{a.cfg.CookieName, a.cfg.CookieName + "_tx"} {
		http.SetCookie(rw, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			Domain:   a.cfg.CookieDomain,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   iamIsSecure(req),
			SameSite: http.SameSiteLaxMode,
		})
	}
	if next := req.URL.Query().Get("next"); next != "" {
		http.Redirect(rw, req, iamSanitizeLocalRedirect(next), http.StatusFound)
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write([]byte(`{"ok":true}`))
}

func (a *iamAuth) redirectURI(req *http.Request) string {
	if a.cfg.RedirectURI != "" {
		return a.cfg.RedirectURI
	}
	scheme := "http"
	if iamIsSecure(req) {
		scheme = "https"
	}
	host := req.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = req.Host
	}
	return scheme + "://" + host + a.cfg.CallbackPath
}

func (a *iamAuth) isSkipped(path string) bool {
	for _, s := range a.cfg.SkipPaths {
		if s == "" {
			continue
		}
		if strings.HasSuffix(s, "/") {
			if strings.HasPrefix(path, s) {
				return true
			}
		} else if path == s {
			return true
		}
	}
	return false
}

func injectIAMHeaders(req *http.Request, p *iamPrincipal) {
	// Always pass the verified Bearer JWT upstream so backends validating JWTs
	// directly work seamlessly alongside backends reading HIP-0026 headers.
	req.Header.Set("Authorization", "Bearer "+p.Token)

	// Canonical HIP-0026 headers (consumed by zap-proto/zip Ctx.User/Org/IsAdmin).
	if p.Sub != "" {
		req.Header.Set("X-User-Id", p.Sub)
		req.Header.Set("X-Hanzo-Sub", p.Sub)
	}
	if p.Username != "" {
		req.Header.Set("X-User-Name", p.Username)
		req.Header.Set("X-Hanzo-User", p.Username)
	}
	if p.Email != "" {
		req.Header.Set("X-User-Email", p.Email)
		req.Header.Set("X-Hanzo-Email", p.Email)
	}
	if p.Org != "" {
		req.Header.Set("X-Org-Id", p.Org)
		req.Header.Set("X-Hanzo-Org", p.Org)
	}
	if p.Owner != "" {
		req.Header.Set("X-User-Owner", p.Owner)
		req.Header.Set("X-Hanzo-Owner", p.Owner)
	}
	if p.Project != "" {
		req.Header.Set("X-Project-Id", p.Project)
	}
	if p.SuperAdmin {
		req.Header.Set("X-User-IsAdmin", "true")
		req.Header.Set("X-Hanzo-SuperAdmin", "true")
	} else {
		req.Header.Set("X-User-IsAdmin", "false")
		req.Header.Set("X-Hanzo-SuperAdmin", "false")
	}
	if p.OrgAdmin {
		req.Header.Set("X-User-IsOrgAdmin", "true")
	} else {
		req.Header.Set("X-User-IsOrgAdmin", "false")
	}
}

func writeIAMJSONError(rw http.ResponseWriter, status int, detail string) {
	rw.Header().Set("Content-Type", "application/problem+json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(status),
		"status": status,
		"detail": detail,
	})
}

func iamBearerToken(h string) string {
	if len(h) < 7 || !strings.EqualFold(h[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(h[7:])
}

func iamWantsHTML(req *http.Request) bool {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return false
	}
	return strings.Contains(req.Header.Get("Accept"), "text/html")
}

func iamIsSecure(req *http.Request) bool {
	if req.TLS != nil {
		return true
	}
	return strings.EqualFold(req.Header.Get("X-Forwarded-Proto"), "https")
}

func iamSanitizeLocalRedirect(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, `/\`) {
		return "/"
	}
	return raw
}

func iamRandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ----- Pure-stdlib JWT + JWKS verification (RS256/RS384/RS512, ES256/ES384/ES512) -----

func (a *iamAuth) verifyToken(ctx context.Context, raw string) (*iamPrincipal, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed jwt")
	}
	hdrBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("invalid jwt header")
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
		return nil, errors.New("invalid jwt header json")
	}
	if !iamAllowedAlg(hdr.Alg) {
		return nil, fmt.Errorf("unsupported jwt alg %q", hdr.Alg)
	}

	pub, err := a.jwks.key(ctx, hdr.Kid)
	if err != nil || pub == nil {
		return nil, errors.New("signing key not found")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("invalid jwt signature encoding")
	}
	if err := iamVerifySig(hdr.Alg, pub, []byte(parts[0]+"."+parts[1]), sig); err != nil {
		return nil, err
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid jwt payload")
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, errors.New("invalid jwt payload json")
	}

	now := time.Now().Unix()
	const leeway = int64(30)
	if exp, ok := iamClaimInt64(claims, "exp"); !ok || now > exp+leeway {
		return nil, errors.New("jwt expired")
	}
	if nbf, ok := iamClaimInt64(claims, "nbf"); ok && now+leeway < nbf {
		return nil, errors.New("jwt not yet valid")
	}
	if a.cfg.Issuer != "" {
		iss := strings.TrimRight(iamClaimString(claims, "iss"), "/")
		if iss != a.cfg.Issuer {
			return nil, errors.New("jwt issuer mismatch")
		}
	}
	if a.cfg.Audience != "" && !iamClaimHasAud(claims, a.cfg.Audience) {
		return nil, errors.New("jwt audience mismatch")
	}

	owner := iamClaimString(claims, "owner")
	org := iamClaimString(claims, "org")
	if org == "" {
		org = iamClaimString(claims, "organization")
	}
	if org == "" {
		org = owner
	}
	username := iamClaimString(claims, "preferred_username")
	if username == "" {
		username = iamClaimString(claims, "name")
	}
	project := iamClaimString(claims, "project")
	if project == "" {
		project = iamClaimString(claims, "project_id")
	}

	return &iamPrincipal{
		Sub:        iamClaimString(claims, "sub"),
		Org:        org,
		Owner:      owner,
		Username:   username,
		Email:      iamClaimString(claims, "email"),
		Project:    project,
		SuperAdmin: owner == "admin",
		OrgAdmin:   iamClaimBool(claims, "isAdmin"),
		Token:      raw,
	}, nil
}

func iamAllowedAlg(alg string) bool {
	switch alg {
	case "RS256", "RS384", "RS512", "ES256", "ES384", "ES512":
		return true
	}
	return false
}

func iamVerifySig(alg string, pub crypto.PublicKey, input, sig []byte) error {
	var h hash.Hash
	var ch crypto.Hash
	switch alg {
	case "RS256", "ES256":
		h, ch = sha256.New(), crypto.SHA256
	case "RS384", "ES384":
		h, ch = sha512.New384(), crypto.SHA384
	case "RS512", "ES512":
		h, ch = sha512.New(), crypto.SHA512
	default:
		return errors.New("unsupported alg")
	}
	_, _ = h.Write(input)
	digest := h.Sum(nil)

	if strings.HasPrefix(alg, "RS") {
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return errors.New("key type mismatch for RSA")
		}
		return rsa.VerifyPKCS1v15(rsaPub, ch, digest, sig)
	}
	ecPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("key type mismatch for ECDSA")
	}
	keySize := (ecPub.Curve.Params().BitSize + 7) / 8
	if len(sig) != 2*keySize {
		return errors.New("invalid ECDSA signature length")
	}
	r := new(big.Int).SetBytes(sig[:keySize])
	s := new(big.Int).SetBytes(sig[keySize:])
	if !ecdsa.Verify(ecPub, digest, r, s) {
		return errors.New("invalid ECDSA signature")
	}
	return nil
}

func iamClaimString(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func iamClaimBool(m map[string]any, k string) bool {
	b, _ := m[k].(bool)
	return b
}

func iamClaimInt64(m map[string]any, k string) (int64, bool) {
	v, ok := m[k]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}

func iamClaimHasAud(m map[string]any, want string) bool {
	switch v := m["aud"].(type) {
	case string:
		return v == want
	case []any:
		for _, elem := range v {
			if s, ok := elem.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

type iamJWKSCache struct {
	url    string
	client *http.Client
	ttl    time.Duration

	mu        sync.RWMutex
	keys      map[string]crypto.PublicKey
	single    crypto.PublicKey
	fetchedAt time.Time
}

func newIAMJWKSCache(jwksURL string, client *http.Client, ttl time.Duration) *iamJWKSCache {
	return &iamJWKSCache{
		url:    jwksURL,
		client: client,
		ttl:    ttl,
		keys:   map[string]crypto.PublicKey{},
	}
}

func (c *iamJWKSCache) key(ctx context.Context, kid string) (crypto.PublicKey, error) {
	c.mu.RLock()
	fresh := time.Since(c.fetchedAt) < c.ttl
	var found crypto.PublicKey
	if fresh {
		if kid != "" {
			found = c.keys[kid]
		} else {
			found = c.single
		}
	}
	c.mu.RUnlock()
	if found != nil {
		return found, nil
	}
	if err := c.refresh(ctx); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if kid != "" {
		if k, ok := c.keys[kid]; ok {
			return k, nil
		}
		if len(c.keys) == 1 && c.single != nil {
			return c.single, nil
		}
		return nil, fmt.Errorf("kid %q not found in JWKS", kid)
	}
	if c.single != nil {
		return c.single, nil
	}
	return nil, errors.New("no usable key in JWKS")
}

func (c *iamJWKSCache) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	var doc struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return err
	}
	parsed := make(map[string]crypto.PublicKey, len(doc.Keys))
	var first crypto.PublicKey
	for _, k := range doc.Keys {
		var pub crypto.PublicKey
		switch k.Kty {
		case "RSA":
			nb, err1 := base64.RawURLEncoding.DecodeString(k.N)
			eb, err2 := base64.RawURLEncoding.DecodeString(k.E)
			if err1 != nil || err2 != nil || len(nb) == 0 || len(eb) == 0 {
				continue
			}
			eVal := int(new(big.Int).SetBytes(eb).Int64())
			if eVal <= 0 {
				continue
			}
			pub = &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: eVal}
		case "EC":
			var curve elliptic.Curve
			switch k.Crv {
			case "P-256":
				curve = elliptic.P256()
			case "P-384":
				curve = elliptic.P384()
			case "P-521":
				curve = elliptic.P521()
			default:
				continue
			}
			xb, err1 := base64.RawURLEncoding.DecodeString(k.X)
			yb, err2 := base64.RawURLEncoding.DecodeString(k.Y)
			if err1 != nil || err2 != nil {
				continue
			}
			pub = &ecdsa.PublicKey{
				Curve: curve,
				X:     new(big.Int).SetBytes(xb),
				Y:     new(big.Int).SetBytes(yb),
			}
		default:
			continue
		}
		if first == nil {
			first = pub
		}
		if k.Kid != "" {
			parsed[k.Kid] = pub
		}
	}
	c.mu.Lock()
	c.keys = parsed
	if len(parsed) == 1 {
		for _, k := range parsed {
			c.single = k
		}
	} else {
		c.single = first
	}
	c.fetchedAt = time.Now()
	c.mu.Unlock()
	return nil
}
