// Copyright 2025 Hanzo AI Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package acme

import (
	"context"
	"errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"testing"
	"time"

	"github.com/hanzoai/ingress/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// Two replicas over ONE fake API server, which is the whole point: the bug being
// fixed is invisible to a single-replica test, and a mocked store would not have
// the conflict semantics that make the fix work.
func twoReplicas(t *testing.T) (a, b *SharedStore) {
	t.Helper()
	client := fake.NewSimpleClientset()
	// One seal for both, which is what two replicas of one deployment have:
	// the same org, the same key from KMS. Running these sealed is not extra
	// coverage, it is the deployment — under the identity seal every document
	// reports no write count and the freshness rule is never reached.
	seal := sealFor(t, false)
	cfg := func(self string) SharedStoreConfig {
		return SharedStoreConfig{Namespace: "hanzo", Self: self, LeaseDuration: time.Minute, Seal: seal}
	}
	var err error
	if a, err = newSharedStore(client, cfg("ingress-0")); err != nil {
		t.Fatal(err)
	}
	if b, err = newSharedStore(client, cfg("ingress-1")); err != nil {
		t.Fatal(err)
	}
	return a, b
}

func certs(domains ...string) []*CertAndStore {
	out := make([]*CertAndStore, 0, len(domains))
	for _, d := range domains {
		out = append(out, &CertAndStore{Certificate: Certificate{Domain: types.Domain{Main: d}}})
	}
	return out
}

// EXACTLY ONE replica may write, and the other must be told so rather than
// silently succeeding into its own copy — which is what the file store did.
func TestOnlyOneReplicaIsTheWriter(t *testing.T) {
	a, b := twoReplicas(t)
	ctx := context.Background()

	if !a.IsWriter(ctx) {
		t.Fatal("the first replica to ask must become the writer")
	}
	if b.IsWriter(ctx) {
		t.Fatal("a second replica claimed the writer role while the first held it")
	}

	if err := a.SaveCertificates("letsencrypt", certs("a.hanzo.ai")); err != nil {
		t.Fatalf("owner write: %v", err)
	}
	err := b.SaveCertificates("letsencrypt", certs("b.hanzo.ai"))
	if !errors.Is(err, ErrNotWriter) {
		t.Fatalf("a reader's write must be refused as ErrNotWriter, got %v", err)
	}
}

// The reader SERVES what the writer obtained. This is the property that makes one
// writer acceptable: a host is not stuck behind whichever replica happens to hold
// the lease.
func TestReaderServesTheWritersCertificates(t *testing.T) {
	a, b := twoReplicas(t)
	ctx := context.Background()

	if !a.IsWriter(ctx) {
		t.Fatal("expected the first replica to own the role")
	}
	if err := a.SaveCertificates("letsencrypt", certs("shared.hanzo.ai")); err != nil {
		t.Fatal(err)
	}

	if err := b.refresh(ctx); err != nil {
		t.Fatalf("reader refresh: %v", err)
	}
	got, err := b.GetCertificates("letsencrypt")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Certificate.Domain.Main != "shared.hanzo.ai" {
		t.Fatalf("reader did not see the writer's certificate: %+v", got)
	}
}

// A writer that lost the lease and has not noticed must not be able to clobber the
// new writer's state. Heartbeat liveness cannot stop it; the round can.
func TestDeposedWriterIsRefused(t *testing.T) {
	a, b := twoReplicas(t)
	ctx := context.Background()

	if !a.IsWriter(ctx) {
		t.Fatal("expected a to own first")
	}
	if err := a.SaveCertificates("letsencrypt", certs("first.hanzo.ai")); err != nil {
		t.Fatal(err)
	}

	// Expire the lease so b can legitimately take over, exactly as it would after
	// a's node stopped renewing.
	api := a.client.CoordinationV1().Leases("hanzo")
	l, err := api.Get(ctx, a.leaseName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stale := metav1.NewMicroTime(time.Now().Add(-2 * time.Minute))
	l.Spec.RenewTime = &stale
	if _, err := api.Update(ctx, l, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	if !b.IsWriter(ctx) {
		t.Fatal("b must take over an expired lease")
	}
	if err := b.SaveCertificates("letsencrypt", certs("second.hanzo.ai")); err != nil {
		t.Fatalf("new writer must be able to write: %v", err)
	}

	// a is still running and still believes it writes. Its round is now behind.
	err = a.SaveCertificates("letsencrypt", certs("clobber.hanzo.ai"))
	if err == nil {
		t.Fatal("the deposed writer's write was ADMITTED — this is the split-brain the round exists to prevent")
	}

	// And the new writer's data survived intact.
	if err := b.refresh(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := b.GetCertificates("letsencrypt")
	if len(got) != 1 || got[0].Certificate.Domain.Main != "second.hanzo.ai" {
		t.Fatalf("the winner's state was damaged: %+v", got)
	}
}

// The round must MOVE on handoff, or a deposed writer's round still matches and
// the refusal above cannot happen.
func TestRoundStrictlyIncreasesOnHandoff(t *testing.T) {
	a, b := twoReplicas(t)
	ctx := context.Background()

	first, err := a.leases.Acquire(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	// A renewal by the SAME holder keeps the round: the owner re-states the same
	// object on every pass, and bumping per write would make its own last write
	// look stale.
	again, err := a.leases.Acquire(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if again.Round != first.Round {
		t.Fatalf("a stable owner's round moved: %d -> %d", first.Round, again.Round)
	}

	api := a.client.CoordinationV1().Leases("hanzo")
	l, _ := api.Get(ctx, a.leaseName, metav1.GetOptions{})
	stale := metav1.NewMicroTime(time.Now().Add(-2 * time.Minute))
	l.Spec.RenewTime = &stale
	if _, err := api.Update(ctx, l, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	taken, err := b.leases.Acquire(ctx, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if taken.Round <= first.Round {
		t.Fatalf("handoff did not advance the round: %d -> %d", first.Round, taken.Round)
	}
}

// A rescheduled pod must start with the estate already known. This is the
// re-issuance storm: ~70 names against a 50/week ceiling, triggered by a restart.
func TestFreshReplicaInheritsStateAndOrdersNothing(t *testing.T) {
	a, _ := twoReplicas(t)
	ctx := context.Background()

	if !a.IsWriter(ctx) {
		t.Fatal("expected a to own")
	}
	if err := a.SaveCertificates("letsencrypt", certs("one.hanzo.ai", "two.hanzo.ai", "three.hanzo.ai")); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveAccount("letsencrypt", &Account{Email: "dev@hanzo.ai"}); err != nil {
		t.Fatal(err)
	}

	// A brand-new pod: same cluster, no local state of any kind. It reads the
	// same sealing key from KMS that its siblings did, which is what makes the
	// document theirs to read.
	fresh, err := newSharedStore(a.client, SharedStoreConfig{
		Namespace: "hanzo", Self: "ingress-2", LeaseDuration: time.Minute, Seal: a.seal,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := fresh.GetCertificates("letsencrypt")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("a fresh replica saw %d certificates, not 3 — it would re-order the estate", len(got))
	}
	// The ACCOUNT matters as much: without it a takeover has to register anew and
	// re-order everything it already holds.
	acct, err := fresh.GetAccount("letsencrypt")
	if err != nil {
		t.Fatal(err)
	}
	if acct == nil || acct.Email != "dev@hanzo.ai" {
		t.Fatalf("a fresh replica did not inherit the ACME account: %+v", acct)
	}
}

// An interleaved read-modify-write must not lose data. The precondition is what
// makes that true, so a conflict has to be retried rather than swallowed.
func TestConcurrentWriteConflictIsRetriedNotLost(t *testing.T) {
	a, _ := twoReplicas(t)
	ctx := context.Background()
	if !a.IsWriter(ctx) {
		t.Fatal("expected a to own")
	}
	if err := a.SaveCertificates("letsencrypt", certs("first.hanzo.ai")); err != nil {
		t.Fatal(err)
	}

	// One injected conflict, as if another writer slipped in between the read and
	// the update, then success.
	client := a.client.(*fake.Clientset)
	var once bool
	client.PrependReactor("update", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		if !once {
			once = true
			return true, nil, kerrors.NewConflict(
				schema.GroupResource{Resource: "secrets"}, a.name, errors.New("modified"))
		}
		return false, nil, nil
	})

	if err := a.SaveCertificates("letsencrypt", certs("first.hanzo.ai", "second.hanzo.ai")); err != nil {
		t.Fatalf("a benign conflict must be retried, not surfaced: %v", err)
	}
	if !once {
		t.Fatal("the conflict reactor never fired — the test proved nothing")
	}
	got, _ := a.GetCertificates("letsencrypt")
	if len(got) != 2 {
		t.Fatalf("the retry lost data: %d certificates, want 2", len(got))
	}
}

// A store that cannot reach the cluster must FAIL, never fall back to per-node
// state — the fallback would reappear exactly when replicas are most likely to
// disagree.
func TestMissingIdentityFailsRatherThanGuessing(t *testing.T) {
	if _, err := newSharedStore(fake.NewSimpleClientset(), SharedStoreConfig{Self: "x"}); err == nil {
		t.Fatal("a store with no namespace was accepted")
	}
}

// A reader must order NOTHING. This is the rate-limit half of the fix: two
// replicas each ordering every name is what turns a reschedule into a week
// without new certificates, and it is also what makes two DNS-01 challenges race
// to write and clean up the same TXT record.
func TestReaderOrdersNothing(t *testing.T) {
	a, b := twoReplicas(t)
	ctx := context.Background()

	if !a.IsWriter(ctx) {
		t.Fatal("expected a to own the role")
	}
	if b.IsWriter(ctx) {
		t.Fatal("expected b to be a reader")
	}

	// A Provider with NOTHING configured — no tlsManager, no certificates. The
	// reader must return before it reads any of that, which is what makes this
	// meaningful: the gate is the first thing in the function, so a reader spends
	// none of the expensive half.
	reader := &Provider{Store: b}
	if got := reader.getUncheckedDomains(ctx, []string{"new.hanzo.ai"}, "default"); len(got) != 0 {
		t.Fatalf("a reader returned %v to order — it must return nothing", got)
	}

	// The writer is deliberately NOT exercised through the same call here: past the
	// gate it dereferences a tlsManager this bare Provider does not have, so it
	// would panic on nil for a reason that has nothing to do with the gate. What
	// matters, and is asserted above, is that the predicate the gate consults
	// answers differently for the two — and the reader's early return proves the
	// gate is reached before any of that state.
	if !a.IsWriter(ctx) || b.IsWriter(ctx) {
		t.Fatal("the gate predicate does not distinguish writer from reader")
	}
}

// LocalStore must NOT be gated. A single-replica deployment has no election to
// consult, and asking one would invent a failure mode where none existed — so the
// optional interface is the mechanism, and this pins it.
func TestLocalStoreIsNotGated(t *testing.T) {
	// A typed nil is enough: the assertion is about the TYPE's method set, so this
	// needs no constructor and no filesystem.
	var s Store = (*LocalStore)(nil)
	if _, ok := s.(writerGate); ok {
		t.Fatal("LocalStore implements writerGate — a single replica would now need an election to order anything")
	}
}
