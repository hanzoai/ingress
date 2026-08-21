package main

import (
	"context"
	"os"
	"time"

	"github.com/hanzoai/ingress/pkg/kms"
	"github.com/hanzoai/ingress/pkg/provider/acme"
	"github.com/rs/zerolog/log"
)

// The edge's own secrets, and where they come from.
//
// Two of them decide whether the estate's TLS is the estate's: the key the ACME
// state is sealed under, and the DNS credential the ACME challenge is answered
// with. Both used to be a base64 field in a Kubernetes Secret, mounted into the
// process environment, and one of them was the Cloudflare account-GLOBAL API key
// — a credential that can edit every zone, read billing and deploy workers, sat
// in an env var of a DaemonSet on every node.
//
// Now they are fetched from KMS at startup, held in memory, and the manifests
// reference no Secret at all.

// secretTimeout bounds startup on an unreachable KMS. It is short because the
// answer to "KMS is down" is to fail and be restarted, not to wait: the edge
// cannot issue a certificate without these, and a pod stuck in a long dial is a
// pod that is not reporting why.
const secretTimeout = 20 * time.Second

// edgeSeal is how the ACME state reaches its store.
//
// KMS configured is the answer: the state is sealed and a KMS that cannot be
// reached is FATAL, never a quiet downgrade to writing private keys in the
// clear. That downgrade is the only failure mode worth refusing to have — it
// would happen exactly when nobody is watching, and it leaves the account key
// readable on a node forever after.
//
// KMS not configured is what every deployment did before this existed: plain
// JSON at mode 0600. It says so, at warn, once, naming the file — so an
// unsealed edge is a thing someone can see in the logs rather than a thing
// nobody knew.
func edgeSeal(client *kms.Client) acme.Seal {
	if client == nil {
		log.Warn().Msg("ACME state will be written unsealed: set INGRESS_KMS_ENDPOINT so the account key and every leaf private key are encrypted at rest")
		return acme.Plain()
	}
	ctx, cancel := context.WithTimeout(context.Background(), secretTimeout)
	defer cancel()

	seal, err := kms.SealFrom(ctx, client, kms.AccountSeal)
	if err != nil {
		log.Fatal().Err(err).Str("org", client.Org()).Str("secret", kms.AccountSeal).
			Msg("ACME sealing key could not be read; refusing to store private keys in the clear")
	}
	log.Info().Str("org", client.Org()).Msg("ACME state is sealed with a key held only in memory")
	return seal
}

// Cloudflare's own env names, in the two families lego reads. lego tries the
// account-global pair FIRST and only falls back to the token, so clearing the
// pair is not tidiness — it is what makes the scoped token take effect.
const (
	envCFToken       = "CLOUDFLARE_DNS_API_TOKEN"
	envCFGlobalKey   = "CLOUDFLARE_API_KEY"
	envCFGlobalEmail = "CLOUDFLARE_EMAIL"
	envCFAltKey      = "CF_API_KEY"
	envCFAltEmail    = "CF_API_EMAIL"
)

// loadDNSCredential puts the DNS-01 credential where lego reads it and takes
// the account-global key away from it.
//
// lego offers no way to hand a provider its credential directly —
// dns.NewDNSChallengeProviderByName resolves everything from the environment —
// so the credential passes through this process's env either way. What is ours
// to decide is WHICH credential: a zone-scoped DNS:Edit token whose worst case
// is a DNS record, rather than an account-global key whose worst case is the
// account.
//
// The clear runs whether or not KMS answered. A pod that still has the legacy
// pair injected from somewhere drops it here rather than carrying it.
func loadDNSCredential(client *kms.Client) {
	defer func() {
		for _, name := range []string{envCFGlobalKey, envCFGlobalEmail, envCFAltKey, envCFAltEmail} {
			if os.Getenv(name) != "" {
				log.Warn().Str("env", name).Msg("dropping Cloudflare account-global credential from the process environment")
				os.Unsetenv(name)
			}
		}
	}()

	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), secretTimeout)
	defer cancel()

	token, err := client.Get(ctx, kms.DNSToken)
	if err != nil {
		// Not fatal, and deliberately so: the DNS-01 challenge is one of three,
		// and an edge that already holds its certificates keeps serving TLS
		// without ever calling Cloudflare. Refusing to boot here would turn a
		// renewal problem into an outage.
		log.Error().Err(err).Str("org", client.Org()).Str("secret", kms.DNSToken).
			Msg("DNS challenge credential could not be read; DNS-01 issuance will fail until it can")
		return
	}
	os.Setenv(envCFToken, token)
	log.Info().Str("org", client.Org()).Msg("DNS challenge credential loaded from KMS (zone-scoped token)")
}

// edgeSecrets is the one call that reads everything this process needs from
// KMS. It runs before the ACME provider is built, because both of its results
// are inputs to it.
func edgeSecrets() acme.Seal {
	client, err := kms.FromEnv()
	if err != nil {
		log.Fatal().Err(err).Msg("KMS is half-configured; refusing to start")
	}
	loadDNSCredential(client)
	return edgeSeal(client)
}
