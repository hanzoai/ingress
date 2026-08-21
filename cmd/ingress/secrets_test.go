package main

import (
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/hanzoai/ingress/pkg/kms"
)

// Every way a Cloudflare credential can reach lego: the variable, and the
// variable naming a file to read it from.
var spellings = []string{
	"CLOUDFLARE_EMAIL", "CLOUDFLARE_EMAIL_FILE",
	"CLOUDFLARE_API_KEY", "CLOUDFLARE_API_KEY_FILE",
	"CF_API_EMAIL", "CF_API_EMAIL_FILE",
	"CF_API_KEY", "CF_API_KEY_FILE",
	"CLOUDFLARE_DNS_API_TOKEN_FILE",
}

func fill(t *testing.T) {
	t.Helper()
	for _, name := range spellings {
		t.Setenv(name, "supplied-"+name)
	}
	t.Setenv(envCFToken, "")
}

// A token replaces the credential, and it replaces every spelling of it. Left
// behind, the account pair still wins: lego reads it first and a token that is
// present is never reached.
func TestSwap(t *testing.T) {
	fill(t)
	swap("zone-scoped-token")

	if got := os.Getenv(envCFToken); got != "zone-scoped-token" {
		t.Errorf("%s = %q, want the token", envCFToken, got)
	}
	for _, name := range spellings {
		if got := os.Getenv(name); got != "" {
			t.Errorf("%s = %q; it survived the swap", name, got)
		}
	}
}

// No KMS is a deployment that reads no secrets from KMS. Its environment is
// what its manifest filled, and it stays that way.
func TestLoadDNSCredential_NoKMSLeavesTheEnvironment(t *testing.T) {
	fill(t)
	loadDNSCredential(nil, time.Second)
	assertSupplied(t)
}

// A KMS that does not answer leaves the environment too. Removing the account
// pair without a token to put in its place would leave the DNS-01 challenge
// with no credential at all — and it would do it on every node at once.
func TestLoadDNSCredential_UnreachableKMSLeavesTheEnvironment(t *testing.T) {
	fill(t)

	// A TLS server this client does not trust: it connects and the read fails,
	// which is the shape of a KMS that is there and not answering.
	srv := httptest.NewTLSServer(nil)
	defer srv.Close()
	t.Setenv("INGRESS_KMS_ENDPOINT", srv.URL)
	t.Setenv("INGRESS_KMS_CLIENT_ID", "ingress")
	t.Setenv("INGRESS_KMS_CLIENT_SECRET", "shhh")

	client, err := kms.FromEnv()
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	loadDNSCredential(client, 300*time.Millisecond)
	if took := time.Since(start); took > time.Second {
		t.Errorf("the read ran for %v against a 300ms bound", took)
	}
	assertSupplied(t)
}

func assertSupplied(t *testing.T) {
	t.Helper()
	for _, name := range spellings {
		if got := os.Getenv(name); got != "supplied-"+name {
			t.Errorf("%s = %q; the credential this deployment was given did not survive", name, got)
		}
	}
	if got := os.Getenv(envCFToken); got != "" {
		t.Errorf("%s = %q; a token appeared without one being read", envCFToken, got)
	}
}
