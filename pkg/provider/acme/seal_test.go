package acme

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanzoai/ingress/pkg/kms"
	"github.com/hanzoai/ingress/pkg/safe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// These drive the real seal rather than a stand-in. What this package owes is
// that every read and every write goes through the seal, carrying the store's
// name and its write count; the only way to show that end to end is to compose
// it with the implementation production runs.

const accountKey = "PRIVATE-KEY-MATERIAL"

func sealFor(t *testing.T, adopt bool) Seal {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	s, err := kms.NewSeal(k, adopt)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The pin that matters: what lands on the node does not contain the account
// key, and the store still reads its own writes.
func TestLocalStore_SealedAtRest(t *testing.T) {
	file := filepath.Join(t.TempDir(), "acme.json")
	seal := sealFor(t, false)

	s := NewLocalStore(file, safe.NewPool(t.Context()), seal)
	require.NoError(t, s.SaveAccount("letsencrypt", &Account{
		Email:      "dev@hanzo.ai",
		PrivateKey: []byte(accountKey),
	}))
	time.Sleep(100 * time.Millisecond)

	onDisk, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.NotContains(t, string(onDisk), accountKey,
		"the ACME account key reached the node in the clear")
	assert.NotContains(t, string(onDisk), "dev@hanzo.ai")

	// A second store over the same file reads it back — the document survives
	// a restart, under the same key and the same name.
	again := NewLocalStore(file, safe.NewPool(t.Context()), seal)
	account, err := again.GetAccount("letsencrypt")
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Equal(t, []byte(accountKey), account.PrivateKey)
}

// The store this process was pointed at is sealed. One that is not is refused,
// and the certificates already there are left exactly as they were.
func TestLocalStore_RefusesAnUnsealedStore(t *testing.T) {
	file := plaintextStore(t)
	before, err := os.ReadFile(file)
	require.NoError(t, err)

	s := NewLocalStore(file, safe.NewPool(t.Context()), sealFor(t, false))
	_, err = s.GetAccount("letsencrypt")
	require.Error(t, err, "an unsealed store was read as if this edge had written it")

	// And it keeps refusing. A refusal that only holds for the first read
	// leaves the store looking empty to the next one, which is the state an
	// ACME provider answers by ordering the estate again over what is there.
	_, err = s.GetAccount("letsencrypt")
	require.Error(t, err, "the second read of a refused store was allowed through")

	// And it refuses to write, so nothing replaces what is there.
	require.Error(t, s.SaveAccount("letsencrypt", &Account{PrivateKey: []byte("REPLACEMENT")}),
		"a refused store accepted a write")
	time.Sleep(150 * time.Millisecond)

	after, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, before, after, "a refused store was rewritten")
}

// Adopting is the operator saying the store is theirs. It keeps the
// certificates — re-ordering them would spend a Let's Encrypt rate limit across
// every host the estate serves — and the clear copy is replaced on the spot
// rather than at the next renewal sixty days out.
func TestLocalStore_AdoptsAnUnsealedStore(t *testing.T) {
	file := plaintextStore(t)

	s := NewLocalStore(file, safe.NewPool(t.Context()), sealFor(t, true))

	account, err := s.GetAccount("letsencrypt")
	require.NoError(t, err)
	require.NotNil(t, account, "adoption lost the ACME account")
	assert.Equal(t, []byte(accountKey), account.PrivateKey)

	time.Sleep(200 * time.Millisecond)
	onDisk, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.NotContains(t, string(onDisk), accountKey,
		"the clear copy was still on the node after the first read")
}

// A document belongs to the store it was written for, so one file's document
// does not load from another path.
func TestLocalStore_RefusesADocumentFromAnotherPath(t *testing.T) {
	dir := t.TempDir()
	here, there := filepath.Join(dir, "acme.json"), filepath.Join(dir, "other.json")
	seal := sealFor(t, false)

	s := NewLocalStore(here, safe.NewPool(t.Context()), seal)
	require.NoError(t, s.SaveAccount("letsencrypt", &Account{PrivateKey: []byte(accountKey)}))
	time.Sleep(100 * time.Millisecond)

	blob, err := os.ReadFile(here)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(there, blob, 0o600))

	_, err = NewLocalStore(there, safe.NewPool(t.Context()), seal).GetAccount("letsencrypt")
	require.Error(t, err, "a document written for one path opened at another")
}

// One seal backs every store in a process, and its adoption is consumed the
// moment any of them lands a sealed write. So a store adopted and sealed, then a
// second store over a replanted pre-seal snapshot under the SAME seal, is
// refused — the rollback the flag would otherwise re-admit for the life of the
// process.
func TestLocalStore_AdoptionEndsAcrossStoresOnceSealed(t *testing.T) {
	seal := sealFor(t, true)

	first := plaintextStore(t)
	snapshot, err := os.ReadFile(first)
	require.NoError(t, err)

	// A store adopts the plaintext and reseals it — a landed write, which
	// consumes the seal's adoption.
	s := NewLocalStore(first, safe.NewPool(t.Context()), seal)
	_, err = s.GetAccount("letsencrypt")
	require.NoError(t, err)
	time.Sleep(150 * time.Millisecond)
	sealedOnDisk, err := os.ReadFile(first)
	require.NoError(t, err)
	require.NotContains(t, string(sealedOnDisk), accountKey, "the first store did not seal")

	// The same snapshot, replanted at another path, read through the SAME seal.
	replanted := filepath.Join(t.TempDir(), "acme.json")
	require.NoError(t, os.WriteFile(replanted, snapshot, 0o600))
	_, err = NewLocalStore(replanted, safe.NewPool(t.Context()), seal).GetAccount("letsencrypt")
	require.Error(t, err, "a replanted pre-seal snapshot was adopted after the seal had sealed a store")
}

// A store that cannot seal writes NOTHING. Falling through to an unsealed write
// is the one outcome this path must not have.
func TestLocalStore_RefusesToWriteWhenSealingFails(t *testing.T) {
	file := filepath.Join(t.TempDir(), "acme.json")
	s := NewLocalStore(file, safe.NewPool(t.Context()), brokenSeal{})

	require.NoError(t, s.SaveAccount("letsencrypt", &Account{PrivateKey: []byte(accountKey)}))
	time.Sleep(100 * time.Millisecond)

	onDisk, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.NotContains(t, string(onDisk), accountKey,
		"a failed seal fell through to writing the document in the clear")
}

type brokenSeal struct{}

func (brokenSeal) Wrap(string, uint64, []byte) ([]byte, error) { return nil, assert.AnError }

func (brokenSeal) Unwrap(_ string, b []byte) ([]byte, uint64, bool, error) {
	return b, 0, true, nil
}

func (brokenSeal) Persisted() {}

// A shared store keeps re-reading its object, so an earlier copy of it can be
// presented to a running replica. The write count is what makes that copy
// visible as earlier, and a replica keeps what it has rather than stepping back.
func TestSharedStore_RefusesAnEarlierDocument(t *testing.T) {
	client := fake.NewSimpleClientset()
	s, err := newSharedStore(client, SharedStoreConfig{
		Namespace: "hanzo", Self: "ingress-0", LeaseDuration: time.Minute, Seal: sealFor(t, false),
	})
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, s.SaveCertificates("letsencrypt", certs("api.hanzo.ai")))
	first, err := client.CoreV1().Secrets("hanzo").Get(ctx, s.name, metav1.GetOptions{})
	require.NoError(t, err)
	earlier := append([]byte(nil), first.Data[storeKey]...)

	require.NoError(t, s.SaveCertificates("letsencrypt", certs("api.hanzo.ai", "cloud.hanzo.ai")))

	// Put the earlier document back, exactly as it was written — a valid
	// envelope, sealed under this key, for this store, at an earlier write.
	current, err := client.CoreV1().Secrets("hanzo").Get(ctx, s.name, metav1.GetOptions{})
	require.NoError(t, err)
	current.Data[storeKey] = earlier
	_, err = client.CoreV1().Secrets("hanzo").Update(ctx, current, metav1.UpdateOptions{})
	require.NoError(t, err)

	s.mu.Lock()
	s.rv = "" // an empty version carries nothing to compare, so refresh reloads
	s.mu.Unlock()

	require.Error(t, s.refresh(ctx), "an earlier document replaced a later one")

	held, err := s.GetCertificates("letsencrypt")
	require.NoError(t, err)
	assert.Len(t, held, 2, "the replica stepped back to the earlier document")
}

// A document belongs to the object it was written for, so one namespace's
// document does not open in another.
func TestSharedStore_RefusesADocumentFromAnotherNamespace(t *testing.T) {
	client := fake.NewSimpleClientset()
	seal := sealFor(t, false)
	mk := func(ns string) *SharedStore {
		s, err := newSharedStore(client, SharedStoreConfig{
			Namespace: ns, Self: "ingress-0", LeaseDuration: time.Minute, Seal: seal,
		})
		require.NoError(t, err)
		return s
	}
	here, there := mk("hanzo"), mk("lux-system")
	ctx := context.Background()

	require.NoError(t, here.SaveCertificates("letsencrypt", certs("api.hanzo.ai")))
	from, err := client.CoreV1().Secrets("hanzo").Get(ctx, here.name, metav1.GetOptions{})
	require.NoError(t, err)

	_, err = client.CoreV1().Secrets("lux-system").Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: there.name, Namespace: "lux-system"},
		Data:       map[string][]byte{storeKey: from.Data[storeKey]},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	require.Error(t, there.refresh(ctx), "a document written for one namespace opened in another")
}

// A conflict is retried, and under a real seal the retry has to work. The
// count of a write that did NOT land must not become the store's own view of
// where the document is, or the re-read sees a document behind it and every
// write from then on is refused — the writer keeps ordering and stops
// persisting.
func TestSharedStore_RetriesAConflictUnderSeal(t *testing.T) {
	client := fake.NewSimpleClientset()
	s, err := newSharedStore(client, SharedStoreConfig{
		Namespace: "hanzo", Self: "ingress-0", LeaseDuration: time.Minute, Seal: sealFor(t, false),
	})
	require.NoError(t, err)
	require.True(t, s.IsWriter(context.Background()))
	require.NoError(t, s.SaveCertificates("letsencrypt", certs("first.hanzo.ai")))

	// One injected conflict, as if a controller touched the object's metadata
	// between the read and the update, then success.
	var fired bool
	client.PrependReactor("update", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		if !fired {
			fired = true
			return true, nil, kerrors.NewConflict(
				schema.GroupResource{Resource: "secrets"}, s.name, errors.New("modified"))
		}
		return false, nil, nil
	})

	require.NoError(t, s.SaveCertificates("letsencrypt", certs("first.hanzo.ai", "second.hanzo.ai")),
		"a benign conflict must be retried, not surfaced")
	require.True(t, fired, "the conflict reactor never fired — the test proved nothing")

	got, err := s.GetCertificates("letsencrypt")
	require.NoError(t, err)
	assert.Len(t, got, 2, "the retry lost data")

	// And the writer keeps working afterwards, which is what a high-water mark
	// left ahead of the document would have taken away.
	require.NoError(t, s.SaveCertificates("letsencrypt", certs("first.hanzo.ai", "second.hanzo.ai", "third.hanzo.ai")))
	got, err = s.GetCertificates("letsencrypt")
	require.NoError(t, err)
	assert.Len(t, got, 3)
}

// The counter is the whole of the freshness rule, so it is stated once here.
func TestCounter(t *testing.T) {
	var c counter

	require.NoError(t, c.read(5))
	require.NoError(t, c.read(5), "the same document read twice is not a step back")
	require.NoError(t, c.read(9))
	require.Error(t, c.read(8), "a document behind the one already read was accepted")

	// A document with no count makes no claim about being current — it is what
	// an unsealed store is, and it neither advances nor rolls back.
	require.NoError(t, c.read(0), "a document carrying no count was read as a step back")

	// next reserves and does not record, so an attempt that is refused leaves
	// the count where the stored document is.
	assert.Equal(t, uint64(10), c.next())
	assert.Equal(t, uint64(10), c.next(), "next recorded a write that had not landed")
	require.NoError(t, c.read(9), "a reserved-but-unlanded write moved the count")

	c.wrote(10)
	assert.Equal(t, uint64(11), c.next())
	require.Error(t, c.read(9), "the count did not move on a write that landed")
}

// Plain() is what every deployment ran before sealing existed, and it stays
// byte-for-byte what it was: an operator reading acme.json on a node still sees
// the document they always saw. It reports the document sealed because there is
// nothing to adopt — for that deployment this IS the storage format.
func TestPlain_IsTheDocumentItself(t *testing.T) {
	doc := []byte(`{"letsencrypt":{"Account":null,"Certificates":null}}`)
	wrapped, err := Plain().Wrap("/data/acme.json", 1, doc)
	require.NoError(t, err)
	assert.Equal(t, doc, wrapped)

	plain, count, sealed, err := Plain().Unwrap("/data/acme.json", doc)
	require.NoError(t, err)
	assert.True(t, sealed)
	assert.Zero(t, count)
	assert.Equal(t, doc, plain)
}

// plaintextStore writes the document an edge running before sealing existed
// would have left on its node.
func plaintextStore(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "acme.json")
	blob, err := json.MarshalIndent(map[string]*StoredData{"letsencrypt": {
		Account: &Account{Email: "dev@hanzo.ai", PrivateKey: []byte(accountKey)},
	}}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, blob, 0o600))
	return file
}
