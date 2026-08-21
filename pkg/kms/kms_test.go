package kms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeKMS answers the two calls the client makes, and records what it was
// asked, so the test can assert the URL shape rather than trusting it.
type fakeKMS struct {
	token   string
	secrets map[string]string
	gotPath string
	gotEnv  string
	gotAuth string
	logins  int
}

func (f *fakeKMS) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/kms/auth/login", func(w http.ResponseWriter, r *http.Request) {
		f.logins++
		var in struct {
			ClientID     string `json:"clientId"`
			ClientSecret string `json:"clientSecret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]string{"accessToken": f.token}); err != nil {
			t.Error(err)
		}
	})
	mux.HandleFunc("/v1/kms/secrets/", func(w http.ResponseWriter, r *http.Request) {
		f.gotPath = strings.TrimPrefix(r.URL.Path, "/v1/kms/secrets/")
		f.gotEnv = r.URL.Query().Get("env")
		f.gotAuth = r.Header.Get("Authorization")
		value, ok := f.secrets[f.gotPath]
		if !ok {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"secret": map[string]string{"value": value}}); err != nil {
			t.Error(err)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// configured points a client at a test server. It builds the struct rather
// than going through FromEnv because these tests are about what goes over the
// wire, and FromEnv is about what an operator configured — including that the
// endpoint is https, which a test server on loopback is not.
func configured(t *testing.T, endpoint string) *Client {
	t.Helper()
	return &Client{
		endpoint: endpoint,
		clientID: "ingress",
		secret:   "shhh",
		org:      "hanzo",
		env:      "default",
		path:     "ingress",
		http:     &http.Client{Timeout: 5 * time.Second},
	}
}

// The client secret is in the body of the login and the bearer is in a header
// of every read, so a plaintext endpoint is a configuration to fix rather than
// a degraded mode to run in.
func TestFromEnv_RequiresHTTPS(t *testing.T) {
	t.Setenv(envClientID, "ingress")
	t.Setenv(envSecret, "shhh")

	t.Setenv(envEndpoint, "http://kms.hanzo.ai")
	if _, err := FromEnv(); err == nil {
		t.Error("FromEnv accepted a plaintext endpoint")
	}

	t.Setenv(envEndpoint, "https://kms.hanzo.ai")
	c, err := FromEnv()
	if err != nil || c == nil {
		t.Errorf("FromEnv(https) = (%v, %v), want a client", c, err)
	}
}

func TestClient_Get(t *testing.T) {
	f := &fakeKMS{token: "bearer-value", secrets: map[string]string{
		"ingress/" + AccountSeal: strings.Repeat("0f", 32),
	}}
	srv := f.server(t)
	c := configured(t, srv.URL)

	got, err := c.Get(context.Background(), AccountSeal)
	if err != nil {
		t.Fatal(err)
	}
	if got != f.secrets["ingress/"+AccountSeal] {
		t.Errorf("value = %q", got)
	}
	if f.gotPath != "ingress/"+AccountSeal {
		t.Errorf("path = %q, want %q", f.gotPath, "ingress/"+AccountSeal)
	}
	if f.gotEnv != "default" {
		t.Errorf("env = %q, want default", f.gotEnv)
	}
	if f.gotAuth != "Bearer bearer-value" {
		t.Errorf("authorization = %q", f.gotAuth)
	}
}

// A missing secret is an error, never an empty value: an empty sealing key
// would build a seal that protects nothing.
func TestClient_MissingSecretIsAnError(t *testing.T) {
	f := &fakeKMS{token: "t", secrets: map[string]string{}}
	c := configured(t, f.server(t).URL)
	if _, err := c.Get(context.Background(), AccountSeal); err == nil {
		t.Error("a missing secret returned no error")
	}
}

// Unconfigured is not an error — that deployment reads no secrets from KMS and
// the caller decides what that means.
func TestFromEnv_Unconfigured(t *testing.T) {
	t.Setenv(envEndpoint, "")
	c, err := FromEnv()
	if err != nil || c != nil {
		t.Errorf("FromEnv() = (%v, %v), want (nil, nil)", c, err)
	}
}

// Half-configured IS an error: an endpoint with no credentials is a deployment
// that meant to use KMS and would otherwise start without it.
func TestFromEnv_HalfConfiguredIsAnError(t *testing.T) {
	t.Setenv(envEndpoint, "https://kms.hanzo.ai")
	t.Setenv(envClientID, "")
	t.Setenv(envSecret, "")
	t.Setenv("IAM_CLIENT_ID", "")
	t.Setenv("IAM_CLIENT_SECRET", "")
	if _, err := FromEnv(); err == nil {
		t.Error("an endpoint with no credentials started without error")
	}
}

// One spelling for the credential. A pod that also carries another subsystem's
// IAM client must not have it picked up here.
func TestFromEnv_TakesOnlyItsOwnCredential(t *testing.T) {
	t.Setenv(envEndpoint, "https://kms.hanzo.ai")
	t.Setenv(envClientID, "")
	t.Setenv(envSecret, "")
	t.Setenv("IAM_CLIENT_ID", "someone-elses")
	t.Setenv("IAM_CLIENT_SECRET", "someone-elses")
	if _, err := FromEnv(); err == nil {
		t.Error("FromEnv used a credential that was not addressed to it")
	}

	t.Setenv(envClientID, "ingress")
	t.Setenv(envSecret, "shhh")
	t.Setenv(envOrg, "lux")
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.clientID != "ingress" {
		t.Errorf("clientID = %q, want ingress", c.clientID)
	}
	if c.Org() != "lux" {
		t.Errorf("org = %q, want lux", c.Org())
	}
}

// The login body carries the client secret. It must never reach an error
// string, because that string goes to a log.
func TestClient_LoginFailureDoesNotEchoTheBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid credentials","clientSecret":"shhh"}`, http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	c := configured(t, srv.URL)

	_, err := c.Get(context.Background(), AccountSeal)
	if err == nil {
		t.Fatal("a failed login returned no error")
	}
	if strings.Contains(err.Error(), "shhh") {
		t.Errorf("the login error carries the credential: %v", err)
	}
}
