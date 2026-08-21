// Package kms reads this deployment's secrets from Hanzo KMS.
//
// The edge holds two things an attacker wants more than any request it proxies:
// the ACME account key, which can re-issue a certificate for every domain the
// estate serves, and the DNS credential the ACME challenge is solved with. Both
// used to arrive as a base64 field in a Kubernetes Secret and live in the pod's
// environment or in a file on the node. This package is where they come from
// now: fetched once at startup, held in memory, never written down.
//
// It speaks the luxfi/kms HTTP contract — the same two calls the gateway's
// routes loader makes, so the estate has one KMS conversation rather than two:
//
//	POST /v1/kms/auth/login          {clientId, clientSecret} -> {accessToken}
//	GET  /v1/kms/secrets/{path}/{name}?env=  -> {"secret":{"value"}}
//
// The org is the TOKEN's, never a path segment: the bearer minted at login
// carries owner=<org> and the server scopes the read to it. Which credential
// logs in is the one honest place an org belongs.
package kms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Env names. The prefix is this service's, so a pod that also carries another
// subsystem's KMS credentials cannot have them picked up here by accident.
const (
	envEndpoint = "INGRESS_KMS_ENDPOINT"
	envClientID = "INGRESS_KMS_CLIENT_ID"
	envSecret   = "INGRESS_KMS_CLIENT_SECRET"
	envOrg      = "INGRESS_KMS_ORG"
	envEnv      = "INGRESS_KMS_ENV"
	envPath     = "INGRESS_KMS_PATH"
)

// Names of the secrets this service reads, under the configured path. They are
// constants rather than env vars: which secrets the ingress needs is a property
// of the ingress, not of the environment it runs in.
const (
	// AccountSeal is the 32-byte key the ACME state is sealed under, hex or
	// standard base64.
	AccountSeal = "acme-seal"
	// DNSToken is the Cloudflare API token the DNS-01 challenge is solved with.
	// It is a ZONE-scoped DNS:Edit token, not the account-global API key.
	DNSToken = "cloudflare-token"
)

// Client reads secrets for one org, at one path, in one environment.
type Client struct {
	endpoint string
	clientID string
	secret   string
	org      string
	env      string
	path     string
	http     *http.Client
}

// FromEnv builds the client this deployment is configured for. It returns
// (nil, nil) when INGRESS_KMS_ENDPOINT is unset — that deployment reads no
// secrets from KMS and the caller decides what that means — and an error when
// the endpoint is set but the credentials to use it are not, which is a
// half-configured deployment rather than an unconfigured one.
func FromEnv() (*Client, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(os.Getenv(envEndpoint)), "/")
	if endpoint == "" {
		return nil, nil
	}
	id := firstOf(os.Getenv(envClientID), os.Getenv("IAM_CLIENT_ID"))
	secret := firstOf(os.Getenv(envSecret), os.Getenv("IAM_CLIENT_SECRET"))
	if id == "" || secret == "" {
		return nil, fmt.Errorf("kms: %s is set but %s/%s are empty", envEndpoint, envClientID, envSecret)
	}
	return &Client{
		endpoint: endpoint,
		clientID: id,
		secret:   secret,
		org:      firstOf(os.Getenv(envOrg), "hanzo"),
		env:      firstOf(os.Getenv(envEnv), "default"),
		path:     firstOf(os.Getenv(envPath), "ingress"),
		http:     &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// Org reports which org's secrets this client reads. Logged at startup so a
// deployment that authenticated as the wrong tenant says so before it serves.
func (c *Client) Org() string { return c.org }

// Get returns the value of one secret under this client's path.
func (c *Client) Get(ctx context.Context, name string) (string, error) {
	token, err := c.login(ctx)
	if err != nil {
		return "", fmt.Errorf("kms: auth: %w", err)
	}
	// Each segment is escaped on its own: escaping the joined string would
	// encode the separators away and the server would read one long name.
	segs := strings.Split(c.path+"/"+name, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	u := fmt.Sprintf("%s/v1/kms/secrets/%s?env=%s",
		c.endpoint, strings.Join(segs, "/"), url.QueryEscape(c.env))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("kms: get %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// The body can carry the server's own error text but never the secret,
		// so it is safe to quote and it is the only way to tell a missing path
		// from a scope the token does not hold.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("kms: get %s returned %d: %s", name, resp.StatusCode, bytes.TrimSpace(body))
	}
	var out struct {
		Secret struct {
			Value string `json:"value"`
		} `json:"secret"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", fmt.Errorf("kms: decode %s: %w", name, err)
	}
	if out.Secret.Value == "" {
		return "", fmt.Errorf("kms: %s/%s is empty", c.path, name)
	}
	return out.Secret.Value, nil
}

func (c *Client) login(ctx context.Context) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"clientId":     c.clientID,
		"clientSecret": c.secret,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/v1/kms/auth/login", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Deliberately no body here: a login response can echo the credential
		// that failed, and this error is going to a log.
		return "", fmt.Errorf("login returned %d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", errors.New("login returned no access token")
	}
	return out.AccessToken, nil
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
