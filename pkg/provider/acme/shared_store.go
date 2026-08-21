// Copyright 2025 Hanzo AI Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package acme

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/hanzoai/ha"
	"github.com/rs/zerolog/log"
	coordv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// SharedStore is the ACME store for MORE THAN ONE REPLICA: one writer, the rest
// readers, over a single object every replica can see.
//
// # The defect it exists to fix
//
// LocalStore keeps acme.json on a filesystem. With replicas > 1 on a hostPath
// that is one FILE PER NODE, and nothing about the design notices: each replica
// resolves independently, writes its own file, and the two drift apart. Measured
// on this fleet — 71 certificates on one replica, 63 on the other, EIGHT hosts
// present on exactly one. Those hosts serve the right certificate or a wrong one
// depending on which replica the load balancer picked, and with sniStrict a
// missing certificate is not a downgrade but a failed handshake. So roughly half
// of connections to a host only one replica holds simply do not complete.
//
// Duplicating the work is the second half of it. Both replicas order every name,
// so a rescheduled pod with an empty hostPath re-orders the whole estate — ~70
// certificates against Let's Encrypt's 50-per-week registered-domain ceiling,
// which is how a routine restart turns into a week without new certificates.
//
// # One writer, and the rest read
//
// WHO writes is decided by github.com/hanzoai/ha, the fleet's one election, at
// the single key "acme": with one key an HRW owner IS a single leader, which is
// the one shape where that word is accurate. A replica that does not own the key
// never orders and never writes — it READS, so it serves every certificate the
// owner obtained without duplicating a single order.
//
// The round comes from a Kubernetes Lease, and it is not an approximation of one:
// Lease.spec.leaseTransitions is a counter the API server increments whenever the
// holder changes, which is verbatim ha's requirement — "monotone non-decreasing
// per key across ALL callers, and STRICTLY increases whenever the writer role
// changes hands". The API server is the linearizable source ha's doc says the
// seam is shaped to accept; no consensus of our own, no second coordination
// system, nothing to operate.
//
// # Why a deposed writer is harmless rather than unlikely
//
// A lease can expire while its holder is still running — a paused process, a
// partitioned node — so leadership alone is not safety. Every write carries the
// round it was made under, and a write whose round is BELOW the round already
// recorded is refused. That is the storage half: the API server's own
// resourceVersion precondition makes the read-modify-write atomic, so two
// replicas cannot interleave, and the round makes the loser's data stale rather
// than merely late. A writer that has lost the lease and does not know it yet
// cannot damage the winner's state.
//
// # Rolling upgrade, with no re-issuance and no gap
//
// Because state lives in the object and not on a node, a new pod starts with
// every certificate already known and orders nothing. It serves immediately, and
// if it later takes the lease it continues from where its predecessor stopped.
// The old pod, drained mid-lease, loses the round rather than the data. That is
// the whole of zero-downtime here: no re-issuance storm on reschedule, no window
// where a replica is serving from an empty store.
//
// Readers poll rather than watch. A poll cannot miss an event it was not
// connected for, needs no reconnect logic, and is bounded by the interval; a
// watch is faster and is the thing that silently stops delivering. The cost of
// being one interval late is that a brand-new host is served by the owner a few
// seconds before its readers — which is what the interval is for.
type SharedStore struct {
	client    kubernetes.Interface
	namespace string
	name      string // Secret holding the ACME state
	leaseName string // Lease naming the writer
	self      string // this replica's stable identity (pod name)
	seal      Seal   // how the state document reaches the Secret

	leaseDuration time.Duration
	pollInterval  time.Duration
	count         counter

	mu     sync.RWMutex
	cache  map[string]*StoredData // resolver -> state, last read
	rv     string                 // resourceVersion the cache was read at ("" when the server sets none)
	loaded bool                   // the cache has been filled at least once
	leases ha.Leases              // the round source
}

// storeKey is the Secret key holding the serialized per-resolver state. One key,
// because the state is one document: a partial write must not be able to leave an
// account without its certificates.
const storeKey = "acme.json"

// roundKey records the round the current contents were written under, and
// holderKey the identity that wrote them. Both live in the SAME object as the
// data, so a reader cannot see one without the others and a writer cannot update
// the data without restating who it was and under what round.
//
// The holder is not redundant with the round. A round fences a writer that is
// BEHIND; it says nothing about two writers who believe they hold the same round,
// which is reachable because the Lease and this Secret are two objects and
// Kubernetes has no transaction spanning them. Refusing a same-round write from a
// different identity closes that: the object itself decides, and it decides with
// one atomic compare-and-swap rather than a handshake across two.
const roundKey = "acme.round"
const holderKey = "acme.holder"

// ErrNotWriter is returned by a write on a replica that does not own the key. It
// is not an error condition — it is the correct outcome for a reader, and the
// caller logs it at debug and carries on.
var ErrNotWriter = errors.New("acme: this replica is not the ACME writer")

// ErrStaleRound is returned when a write's round is below the round already
// recorded. The writer has been deposed and has not noticed yet; refusing is what
// makes that harmless.
var ErrStaleRound = errors.New("acme: write refused, round superseded")

// SharedStoreConfig is the operator-facing shape. Zero values are filled with
// defaults that are safe for two replicas, so the only required field is what
// cannot be guessed: which namespace to write in.
type SharedStoreConfig struct {
	Namespace     string
	SecretName    string
	LeaseName     string
	Self          string
	LeaseDuration time.Duration
	PollInterval  time.Duration

	// Seal is how the state document reaches the Secret. A Kubernetes Secret
	// is base64, not encryption: whoever can read the object reads the ACME
	// account key and every leaf private key in it. Zero value is Plain(),
	// which is that.
	Seal Seal
}

// NewSharedStore builds the store from in-cluster credentials.
//
// It fails rather than degrading to a local file. A store that silently falls
// back to per-node state is the defect this type exists to remove, and it would
// reappear exactly when the cluster is unreachable — the moment when two replicas
// are most likely to disagree.
func NewSharedStore(cfg SharedStoreConfig) (*SharedStore, error) {
	restCfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("acme shared store needs in-cluster credentials: %w", err)
	}
	client, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("acme shared store: build client: %w", err)
	}
	return newSharedStore(client, cfg)
}

// newSharedStore is the injectable half, so a test drives a fake clientset and
// the defaults are exercised by the same code production runs.
func newSharedStore(client kubernetes.Interface, cfg SharedStoreConfig) (*SharedStore, error) {
	if cfg.Namespace == "" {
		cfg.Namespace = os.Getenv("POD_NAMESPACE")
	}
	if cfg.Namespace == "" {
		return nil, errors.New("acme shared store: namespace is required (set POD_NAMESPACE)")
	}
	if cfg.Self == "" {
		cfg.Self = os.Getenv("POD_NAME")
	}
	if cfg.Self == "" {
		// Hostname is the pod name under Kubernetes, and it is stable for the
		// pod's life — which is what ha requires of an identity.
		if h, err := os.Hostname(); err == nil {
			cfg.Self = h
		}
	}
	if cfg.Self == "" {
		return nil, errors.New("acme shared store: a stable self identity is required (set POD_NAME)")
	}
	if cfg.SecretName == "" {
		cfg.SecretName = "ingress-acme"
	}
	if cfg.LeaseName == "" {
		cfg.LeaseName = "ingress-acme"
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = 30 * time.Second
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.Seal == nil {
		cfg.Seal = Plain()
	}
	s := &SharedStore{
		client:        client,
		namespace:     cfg.Namespace,
		name:          cfg.SecretName,
		leaseName:     cfg.LeaseName,
		self:          cfg.Self,
		leaseDuration: cfg.LeaseDuration,
		pollInterval:  cfg.PollInterval,
		seal:          cfg.Seal,
		cache:         map[string]*StoredData{},
	}
	s.leases = &kubeLeases{store: s}
	return s, nil
}

// id names the object this store keeps its state in. It is what the seal binds
// the ciphertext to, so a document written for this Secret opens for this
// Secret and for nothing else — not for another namespace's, and not for a
// file on a node.
func (s *SharedStore) id() string { return s.namespace + "/" + s.name }

// IsWriter reports whether this replica currently owns the ACME role. The
// ordering path asks it so a reader does not order a certificate it would then be
// refused permission to store — the order is the expensive half (a DNS-01
// round-trip and a rate-limit slot), so declining early is the point.
func (s *SharedStore) IsWriter(ctx context.Context) bool {
	_, err := s.leases.Acquire(ctx, "acme")
	return err == nil
}

// Start begins the reader poll. It returns immediately; the loop exits when ctx
// is done. The owner polls too: it is how it notices a write it did not make,
// which is what a lease handoff looks like from the new owner's side.
func (s *SharedStore) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(s.pollInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := s.refresh(ctx); err != nil && !kerrors.IsNotFound(err) {
					log.Debug().Err(err).Msg("acme shared store: refresh")
				}
			}
		}
	}()
}

// refresh reloads the cache when the object has changed. It compares
// resourceVersion first, so a poll that finds nothing new costs one GET and no
// deserialization.
func (s *SharedStore) refresh(ctx context.Context) error {
	sec, err := s.client.CoreV1().Secrets(s.namespace).Get(ctx, s.name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	// An EMPTY resourceVersion carries no version to compare, so it can only mean
	// reload. Treating it as "unchanged" makes the first read stick forever and a
	// reader never sees a certificate the writer obtained — silently, because
	// nothing errors.
	s.mu.RLock()
	unchanged := sec.ResourceVersion != "" && sec.ResourceVersion == s.rv
	s.mu.RUnlock()
	if unchanged {
		return nil
	}
	data, count, sealed, err := decodeStored(s.seal, s.id(), sec.Data[storeKey])
	if err != nil {
		return err
	}
	if err := s.count.read(count); err != nil {
		return err
	}
	if !sealed {
		// A reader cannot rewrite an adopted store — only the elected writer
		// may — so it says so and the writer's next mutation seals it.
		log.Warn().Str("secret", s.name).
			Msg("ACME state adopted unsealed; the writer will seal it on its next write")
	}
	s.mu.Lock()
	s.cache, s.rv, s.loaded = data, sec.ResourceVersion, true
	s.mu.Unlock()
	return nil
}

// GetAccount returns the resolver's ACME account. Every replica reads it,
// including readers: an account is what makes a certificate renewable, so a
// replica that took over without one would have to register a new account and
// re-order everything it already holds.
func (s *SharedStore) GetAccount(resolverName string) (*Account, error) {
	d, err := s.stateOf(resolverName)
	if err != nil {
		return nil, err
	}
	return d.Account, nil
}

func (s *SharedStore) SaveAccount(resolverName string, account *Account) error {
	return s.mutate(resolverName, func(d *StoredData) { d.Account = account })
}

func (s *SharedStore) GetCertificates(resolverName string) ([]*CertAndStore, error) {
	d, err := s.stateOf(resolverName)
	if err != nil {
		return nil, err
	}
	return d.Certificates, nil
}

// SaveCertificates replaces the resolver's certificate set, which is the shape
// the Store interface asks for. Under one writer that is safe: the owner holds the
// whole set in memory and re-states it. It is NOT safe under two, which is why the
// round check below is the load-bearing line and not a formality.
func (s *SharedStore) SaveCertificates(resolverName string, certificates []*CertAndStore) error {
	return s.mutate(resolverName, func(d *StoredData) { d.Certificates = certificates })
}

// stateOf serves the cache, filling it on first use so a boot before the first
// poll tick is not blind.
func (s *SharedStore) stateOf(resolverName string) (*StoredData, error) {
	s.mu.RLock()
	d, seeded := s.cache[resolverName], s.loaded
	s.mu.RUnlock()
	if !seeded {
		if err := s.refresh(context.Background()); err != nil && !kerrors.IsNotFound(err) {
			return nil, err
		}
		s.mu.RLock()
		d = s.cache[resolverName]
		s.mu.RUnlock()
	}
	if d == nil {
		return &StoredData{}, nil
	}
	return d, nil
}

// mutate is the one write path: acquire the round, read-modify-write under the
// API server's resourceVersion precondition, and refuse a superseded round.
//
// Ordering matters. The round is taken FIRST, so a replica that has lost the
// lease stops before it reads, and the precondition is checked LAST, so two
// writers that raced both cannot win. The retry exists for the benign conflict —
// the owner writing twice in quick succession — and gives up rather than looping,
// because a persistent conflict means someone else is writing and this replica
// should not be.
func (s *SharedStore) mutate(resolverName string, apply func(*StoredData)) error {
	ctx := context.Background()

	lease, err := s.leases.Acquire(ctx, "acme")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNotWriter, err)
	}

	const attempts = 3
	for i := range attempts {
		// Whether the object EXISTS comes from the Get error and nothing else.
		// Inferring it from an empty resourceVersion conflates "absent" with
		// "present but unversioned", and then every write takes the Create branch
		// and fails AlreadyExists forever against an object that is right there.
		var absent bool
		sec, err := s.client.CoreV1().Secrets(s.namespace).Get(ctx, s.name, metav1.GetOptions{})
		switch {
		case kerrors.IsNotFound(err):
			absent = true
			sec = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: s.name, Namespace: s.namespace},
				Data:       map[string][]byte{},
			}
		case err != nil:
			return fmt.Errorf("acme shared store: read: %w", err)
		}

		// Two refusals, not one. BEHIND the recorded round means this writer was
		// deposed and has not noticed. AT the recorded round but under a different
		// identity means two replicas believe they hold the same round — the
		// cross-object window between the Lease and this Secret, which no
		// Kubernetes transaction spans. Either way the older writer's work becomes
		// harmless instead of authoritative.
		recorded, who := roundOf(sec), string(sec.Data[holderKey])
		switch {
		case recorded > lease.Round:
			return fmt.Errorf("%w: at round %d, object records %d", ErrStaleRound, lease.Round, recorded)
		case recorded == lease.Round && who != "" && who != s.self:
			return fmt.Errorf("%w: round %d is held by %s, not %s", ErrStaleRound, recorded, who, s.self)
		}

		state, count, _, err := decodeStored(s.seal, s.id(), sec.Data[storeKey])
		if err != nil {
			return err
		}
		if err := s.count.read(count); err != nil {
			return err
		}
		d := state[resolverName]
		if d == nil {
			d = &StoredData{}
			state[resolverName] = d
		}
		apply(d)

		want := s.count.next()
		blob, err := encodeStored(s.seal, s.id(), want, state)
		if err != nil {
			return err
		}
		if sec.Data == nil {
			sec.Data = map[string][]byte{}
		}
		sec.Data[storeKey] = blob
		sec.Data[roundKey] = []byte(fmt.Sprintf("%d", lease.Round))
		sec.Data[holderKey] = []byte(s.self)

		var out *corev1.Secret
		if absent {
			out, err = s.client.CoreV1().Secrets(s.namespace).Create(ctx, sec, metav1.CreateOptions{})
		} else {
			// Update carries the resourceVersion read above, so the API server
			// rejects it if anything wrote in between. That is the atomicity.
			out, err = s.client.CoreV1().Secrets(s.namespace).Update(ctx, sec, metav1.UpdateOptions{})
		}
		switch {
		case kerrors.IsConflict(err) || kerrors.IsAlreadyExists(err):
			if i == attempts-1 {
				return fmt.Errorf("acme shared store: %w after %d attempts", err, attempts)
			}
			continue
		case err != nil:
			return fmt.Errorf("acme shared store: write: %w", err)
		}

		s.count.wrote(want)
		s.seal.Persisted()
		s.mu.Lock()
		s.cache, s.rv, s.loaded = state, out.ResourceVersion, true
		s.mu.Unlock()
		return nil
	}
	return errors.New("acme shared store: exhausted write attempts")
}

// roundOf reads the recorded round, treating anything unparseable as zero. A
// corrupt round must not wedge writing forever: zero admits the next write, which
// then records a good value.
func roundOf(sec *corev1.Secret) ha.Round {
	var r uint64
	if _, err := fmt.Sscanf(string(sec.Data[roundKey]), "%d", &r); err != nil {
		return 0
	}
	return ha.Round(r)
}

// kubeLeases implements ha.Leases over a Kubernetes Lease.
//
// leaseTransitions is the round: the API server increments it on every handoff,
// so it is monotone across all callers and strictly increases when the role moves
// — ha's invariant, enforced by something that already exists rather than by us.
type kubeLeases struct{ store *SharedStore }

func (k *kubeLeases) Acquire(ctx context.Context, key string) (ha.Lease, error) {
	s := k.store
	api := s.client.CoordinationV1().Leases(s.namespace)
	now := metav1.NewMicroTime(time.Now())
	secs := int32(s.leaseDuration / time.Second)

	cur, err := api.Get(ctx, s.leaseName, metav1.GetOptions{})
	if kerrors.IsNotFound(err) {
		created, cerr := api.Create(ctx, &coordv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: s.leaseName, Namespace: s.namespace},
			Spec: coordv1.LeaseSpec{
				HolderIdentity:       &s.self,
				LeaseDurationSeconds: &secs,
				AcquireTime:          &now,
				RenewTime:            &now,
				LeaseTransitions:     ptr[int32](1),
			},
		}, metav1.CreateOptions{})
		if cerr != nil {
			// Someone else created it first. That is a normal race and the
			// answer is simply that we are not the writer this round.
			return ha.Lease{}, fmt.Errorf("acquire %q: %w", key, cerr)
		}
		return leaseOf(key, s.self, created), nil
	}
	if err != nil {
		return ha.Lease{}, fmt.Errorf("acquire %q: %w", key, err)
	}

	held := cur.Spec.HolderIdentity != nil && *cur.Spec.HolderIdentity == s.self
	expired := cur.Spec.RenewTime == nil ||
		time.Since(cur.Spec.RenewTime.Time) > s.leaseDuration

	if !held && !expired {
		return ha.Lease{}, fmt.Errorf("acquire %q: held by %s", key, holder(cur))
	}

	next := cur.DeepCopy()
	next.Spec.RenewTime = &now
	if !held {
		// Taking over: the round MUST move, so a write from the previous holder
		// is refused from this moment on.
		next.Spec.HolderIdentity = &s.self
		next.Spec.AcquireTime = &now
		next.Spec.LeaseTransitions = ptr(transitions(cur) + 1)
	}

	out, err := api.Update(ctx, next, metav1.UpdateOptions{})
	if err != nil {
		// A conflict means another replica moved the lease first — it is the
		// writer, we are not. Fail closed.
		return ha.Lease{}, fmt.Errorf("acquire %q: %w", key, err)
	}
	return leaseOf(key, s.self, out), nil
}

func leaseOf(key, self string, l *coordv1.Lease) ha.Lease {
	return ha.Lease{
		Key:   key,
		Owner: ha.Member{ID: self},
		Round: ha.Round(transitions(l)),
	}
}

func transitions(l *coordv1.Lease) int32 {
	if l.Spec.LeaseTransitions == nil {
		return 0
	}
	return *l.Spec.LeaseTransitions
}

func holder(l *coordv1.Lease) string {
	if l.Spec.HolderIdentity == nil {
		return "<none>"
	}
	return *l.Spec.HolderIdentity
}

func ptr[T any](v T) *T { return &v }

// writerGate is implemented by a Store that knows whether THIS replica may write.
//
// It is an OPTIONAL interface on purpose: LocalStore does not implement it and
// must not, because a single-replica deployment has no election to consult and
// asking one would be a failure mode invented out of nothing. So the gate is
// present exactly when there is something to gate.
type writerGate interface {
	IsWriter(ctx context.Context) bool
}

// Compile-time proof that SharedStore satisfies both the store contract and the
// gate, so a refactor that drops either is a build failure rather than a silent
// return to every-replica-orders.
var (
	_ Store      = (*SharedStore)(nil)
	_ writerGate = (*SharedStore)(nil)
)
