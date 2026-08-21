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
// One of them decides whether the estate's TLS is the estate's: the key the
// ACME state is sealed under. It is read from KMS at startup and held in
// memory, so the state on the node is inert and the key that opens it is
// nowhere on the node at all.
//
// The DNS credential the ACME challenge is answered with is read from KMS too
// when KMS holds one. When it does not, the credential this deployment was
// given stays exactly as it was given.

// reach bounds how long startup spends reaching KMS.
//
// It is shorter than the window a liveness probe allows a starting container,
// because a retry the kubelet outlives is not a retry — it is a slower way to
// be killed, with the reason in a log nobody correlates. Inside this bound the
// process either has its secrets or says why it does not.
const reach = 30 * time.Second

// envAdopt is the operator's one-time answer to a store that is not sealed.
//
// Unset — everywhere, always, unless someone is standing at the console — an
// unsealed store is refused: this process was pointed at a store it did not
// write, and writing this edge's account key over it is not a recovery. Set,
// this boot opens that store once and writes it back under seal, which is how a
// store that predates sealing keeps its certificates instead of re-ordering
// every one of them against a rate limit.
//
// It does its work once and then does nothing: after that boot the store IS
// sealed, so the unsealed path is never reached again. It says so at warn on
// every boot it is set, so it is not left on quietly.
const envAdopt = "INGRESS_ACME_ADOPT"

// edgeSeal is how the ACME state reaches its store.
//
// KMS configured is the answer: the state is sealed, and a KMS that cannot be
// reached within the bound is fatal rather than a quiet downgrade to writing
// private keys in the clear. That downgrade is the one failure mode worth
// refusing to have — it would happen exactly when nobody is watching, and it
// leaves the account key readable on a node forever after.
//
// KMS not configured is what every deployment did before this existed: plain
// JSON at mode 0600. It says so, at warn, once, naming the file — so an
// unsealed edge is a thing someone can see in the logs rather than a thing
// nobody knew.
func edgeSeal(client *kms.Client, limit time.Duration) acme.Seal {
	if client == nil {
		log.Warn().Msg("ACME state will be written unsealed: set INGRESS_KMS_ENDPOINT so the account key and every leaf private key are encrypted at rest")
		return acme.Plain()
	}

	adopt := os.Getenv(envAdopt) != ""
	if adopt {
		log.Warn().Str("env", envAdopt).
			Msg("an ACME state that is not sealed will be adopted and written back under seal on this boot")
	}

	var seal *kms.Seal
	err := kms.Retry(limit, func(ctx context.Context) error {
		s, err := kms.SealFrom(ctx, client, kms.AccountSeal, adopt)
		seal = s
		return err
	})
	if err != nil {
		log.Fatal().Err(err).Str("org", client.Org()).Str("secret", kms.AccountSeal).
			Dur("tried", limit).
			Msg("ACME sealing key could not be read; refusing to store private keys in the clear")
	}
	log.Info().Str("org", client.Org()).Str("key", seal.ID()).
		Msg("ACME state is sealed with a key held only in memory")
	return seal
}

// envCFToken is the variable lego reads a zone-scoped Cloudflare token from.
const envCFToken = "CLOUDFLARE_DNS_API_TOKEN"

// account is the Cloudflare account-global credential, in both namespaces lego
// accepts it under. It authorizes every zone, billing and workers; a DNS-01
// challenge needs one record in one zone.
//
// lego tries this pair FIRST and only falls back to a token, so the pair and a
// token do not coexist: whichever process holds both uses the pair.
var account = []string{
	"CLOUDFLARE_EMAIL", "CLOUDFLARE_API_KEY",
	"CF_API_EMAIL", "CF_API_KEY",
}

// loadDNSCredential puts a zone-scoped token where lego reads it, in place of
// whatever else could have supplied that credential.
//
// lego offers no way to hand a provider its credential directly —
// dns.NewDNSChallengeProviderByName resolves everything from the environment —
// so the credential passes through this process's env either way. What is ours
// to decide is WHICH credential.
//
// It is a swap, and only a swap. When KMS holds no token, or cannot be reached
// within the bound, the environment is left exactly as the deployment filled
// it: taking the account pair away without a token to put in its place would
// leave the challenge with no credential at all.
func loadDNSCredential(client *kms.Client, limit time.Duration) {
	if client == nil {
		return
	}
	var token string
	err := kms.Retry(limit, func(ctx context.Context) error {
		v, err := client.Get(ctx, kms.DNSToken)
		token = v
		return err
	})
	if err != nil {
		// Not fatal, and deliberately so. The DNS-01 challenge is one of three,
		// an edge that already holds its certificates keeps serving TLS without
		// ever calling Cloudflare, and a deployment may supply this credential
		// to the process directly.
		log.Warn().Err(err).Str("org", client.Org()).Str("secret", kms.DNSToken).
			Msg("no DNS challenge credential in KMS; the challenge uses the credential this deployment was given")
		return
	}

	swap(token)
	log.Info().Str("org", client.Org()).Msg("DNS challenge credential loaded from KMS (zone-scoped token)")
}

// swap puts the token where lego reads it, in place of every other way that
// credential could arrive: the file spelling of the token itself, and the
// account pair lego prefers over any token. The removals go first, so nothing
// removes what was just put in place.
func swap(token string) {
	for _, name := range append([]string{envCFToken}, account...) {
		unset(name)
	}
	os.Setenv(envCFToken, token)
}

// unset removes a credential from the process environment in both spellings
// lego reads it in: the variable, and the variable naming a file to read it
// from (env.GetOrFile). A set that covers one and not the other covers neither.
func unset(name string) {
	for _, n := range []string{name, name + "_FILE"} {
		if os.Getenv(n) == "" {
			continue
		}
		log.Warn().Str("env", n).Msg("dropping a superseded Cloudflare credential from the process environment")
		os.Unsetenv(n)
	}
}

// edgeSecrets is the one call that reads everything this process needs from
// KMS. It runs before the ACME provider is built, because both of its results
// are inputs to it.
func edgeSecrets() acme.Seal {
	client, err := kms.FromEnv()
	if err != nil {
		// A configuration error does not heal, so it is not retried.
		log.Fatal().Err(err).Msg("KMS is configured but not usable; refusing to start")
	}
	loadDNSCredential(client, reach)
	return edgeSeal(client, reach)
}
