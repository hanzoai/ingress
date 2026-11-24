package auth

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hanzoai/ingress/pkg/config/dynamic"
)

func testSignRS256(t *testing.T, priv *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	hdr, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid})
	pay, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(pay)
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func testSignES256(t *testing.T, priv *ecdsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	hdr, _ := json.Marshal(map[string]string{"alg": "ES256", "typ": "JWT", "kid": kid})
	pay, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(pay)
	sum := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, priv, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestIAMAuth_FullOIDCPKCE_JWKS_HeaderInjection_And_SuperAdmin(t *testing.T) {
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	var capturedChallenge string
	var idp *httptest.Server
	idp = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/iam/.well-known/jwks":
			w.Header().Set("Content-Type", "application/json")
			n := base64.RawURLEncoding.EncodeToString(rsaPriv.PublicKey.N.Bytes())
			e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(rsaPriv.PublicKey.E)).Bytes())
			x := base64.RawURLEncoding.EncodeToString(ecPriv.PublicKey.X.Bytes())
			y := base64.RawURLEncoding.EncodeToString(ecPriv.PublicKey.Y.Bytes())
			_ = json.NewEncoder(w).Encode(map[string]any{
				"keys": []map[string]any{
					{"kty": "RSA", "kid": "rsa-1", "use": "sig", "alg": "RS256", "n": n, "e": e},
					{"kty": "EC", "kid": "ec-1", "use": "sig", "alg": "ES256", "crv": "P-256", "x": x, "y": y},
				},
			})
		case "/v1/iam/oauth/token":
			_ = r.ParseForm()
			sum := sha256.Sum256([]byte(r.FormValue("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(sum[:]) != capturedChallenge {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			tok := testSignRS256(t, rsaPriv, "rsa-1", map[string]any{
				"iss":                idp.URL,
				"sub":                "u-100",
				"preferred_username": "alice",
				"email":              "alice@hanzo.ai",
				"owner":              "hanzo",
				"organization":       "hanzo",
				"isAdmin":            true, // org admin, NOT platform SuperAdmin
				"exp":                time.Now().Add(time.Hour).Unix(),
			})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": tok,
				"expires_in":   3600,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer idp.Close()

	var seenHeaders http.Header
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream-ok"))
	})

	mw, err := NewIAM(context.Background(), upstream, dynamic.IAMAuth{
		Issuer:    idp.URL,
		ClientID:  "hanzo-flow",
		SkipPaths: []string{"/healthz", "/public/"},
	}, "iam-edge")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Skipped path passes through, but spoofed identity headers are stripped.
	seenHeaders = nil
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://flow.hanzo.ai/healthz", nil)
	req.Header.Set("X-User-IsAdmin", "true")
	req.Header.Set("X-Hanzo-SuperAdmin", "true")
	req.Header.Set("X-User-Id", "forged")
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/healthz status = %d, want 200", rec.Code)
	}
	if seenHeaders.Get("X-User-IsAdmin") != "" || seenHeaders.Get("X-Hanzo-SuperAdmin") != "" || seenHeaders.Get("X-User-Id") != "" {
		t.Fatalf("spoofed headers leaked upstream: %v", seenHeaders)
	}

	// 2. Unauthenticated API request returns 401 + WWW-Authenticate Bearer.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "https://flow.hanzo.ai/v1/flows", nil)
	req.Header.Set("Accept", "application/json")
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("API unauth status = %d, want 401", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("WWW-Authenticate"), "Bearer ") {
		t.Errorf("missing WWW-Authenticate header: %q", rec.Header().Get("WWW-Authenticate"))
	}

	// 3. Unauthenticated browser request redirects to /auth/login?next=/dashboard.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "https://flow.hanzo.ai/dashboard", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/auth/login?next=%2Fdashboard" {
		t.Fatalf("browser redirect = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	// 4. Full PKCE login -> callback -> session cookie -> authenticated upstream headers.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "https://flow.hanzo.ai/auth/login?next=/dashboard", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("/auth/login status = %d, want 302", rec.Code)
	}
	authURL, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	capturedChallenge = authURL.Query().Get("code_challenge")
	if authURL.Query().Get("code_challenge_method") != "S256" || capturedChallenge == "" {
		t.Fatalf("missing PKCE S256 challenge: %v", authURL.Query())
	}
	state := authURL.Query().Get("state")
	txCookies := rec.Result().Cookies()

	cbRec := httptest.NewRecorder()
	cbReq := httptest.NewRequest(http.MethodGet, "https://flow.hanzo.ai/callback?code=code-xyz&state="+url.QueryEscape(state), nil)
	cbReq.Header.Set("X-Forwarded-Proto", "https")
	for _, ck := range txCookies {
		cbReq.AddCookie(ck)
	}
	mw.ServeHTTP(cbRec, cbReq)
	if cbRec.Code != http.StatusFound || cbRec.Header().Get("Location") != "/dashboard" {
		t.Fatalf("/callback failed: %d loc=%q body=%s", cbRec.Code, cbRec.Header().Get("Location"), cbRec.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, ck := range cbRec.Result().Cookies() {
		if ck.Name == defaultIAMCookieName && ck.Value != "" {
			sessionCookie = ck
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || !sessionCookie.Secure {
		t.Fatalf("expected Secure HttpOnly session cookie, got %v", cbRec.Result().Cookies())
	}

	// Authenticated request with session cookie + spoofed admin header.
	seenHeaders = nil
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "https://flow.hanzo.ai/dashboard", nil)
	req.AddCookie(sessionCookie)
	req.Header.Set("X-User-IsAdmin", "true")
	req.Header.Set("X-Hanzo-SuperAdmin", "true")
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated request status = %d, want 200", rec.Code)
	}
	if seenHeaders.Get("X-User-Id") != "u-100" || seenHeaders.Get("X-Org-Id") != "hanzo" || seenHeaders.Get("X-Hanzo-User") != "alice" {
		t.Errorf("unexpected upstream identity headers: %v", seenHeaders)
	}
	if !strings.HasPrefix(seenHeaders.Get("Authorization"), "Bearer ") {
		t.Errorf("expected Bearer JWT forwarded upstream")
	}
	// Org isAdmin MUST NOT become platform SuperAdmin.
	if seenHeaders.Get("X-User-IsAdmin") != "false" || seenHeaders.Get("X-Hanzo-SuperAdmin") != "false" || seenHeaders.Get("X-User-IsOrgAdmin") != "true" {
		t.Errorf("SuperAdmin predicate violated: isAdmin=%q superAdmin=%q orgAdmin=%q",
			seenHeaders.Get("X-User-IsAdmin"), seenHeaders.Get("X-Hanzo-SuperAdmin"), seenHeaders.Get("X-User-IsOrgAdmin"))
	}

	// 5. ES256 Bearer token with owner == "admin" grants SuperAdmin.
	seenHeaders = nil
	adminJWT := testSignES256(t, ecPriv, "ec-1", map[string]any{
		"iss":                idp.URL,
		"sub":                "u-root",
		"preferred_username": "z",
		"owner":              "admin",
		"organization":       "admin",
		"exp":                time.Now().Add(time.Hour).Unix(),
	})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "https://flow.hanzo.ai/v1/admin", nil)
	req.Header.Set("Authorization", "Bearer "+adminJWT)
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin bearer status = %d, want 200", rec.Code)
	}
	if seenHeaders.Get("X-User-IsAdmin") != "true" || seenHeaders.Get("X-Hanzo-SuperAdmin") != "true" {
		t.Errorf("owner==admin did not grant SuperAdmin: %v", seenHeaders)
	}

	// 6. Expired & alg=none JWTs rejected.
	expJWT := testSignRS256(t, rsaPriv, "rsa-1", map[string]any{
		"iss": idp.URL,
		"sub": "u-old",
		"exp": time.Now().Add(-time.Minute).Unix(),
	})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "https://flow.hanzo.ai/v1/flows", nil)
	req.Header.Set("Authorization", "Bearer "+expJWT)
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expired token status = %d, want 401", rec.Code)
	}

	noneHdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	nonePay := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"` + idp.URL + `","sub":"evil","owner":"admin","exp":9999999999}`))
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "https://flow.hanzo.ai/v1/flows", nil)
	req.Header.Set("Authorization", "Bearer "+noneHdr+"."+nonePay+".")
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("alg=none status = %d, want 401", rec.Code)
	}

	// 7. Logout clears session cookie.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "https://flow.hanzo.ai/auth/logout", nil)
	req.AddCookie(sessionCookie)
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", rec.Code)
	}
	cleared := false
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == defaultIAMCookieName && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("logout did not clear session cookie: %v", rec.Result().Cookies())
	}
}
