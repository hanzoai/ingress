package acme

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanzoai/ingress/pkg/safe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSeal is a stand-in for the KMS-backed one: a reversible transform that is
// obviously not the plaintext, plus the same "a document written before sealing
// was configured comes back with sealed=false" contract the real seal has.
//
// It is deliberately NOT encryption. This package's job is to route every read
// and write through the seal; whether the seal is sound is pkg/kms's job and is
// tested there against AES-256-GCM. A fake here keeps the two tests testing two
// different things.
type testSeal struct{ marker string }

func (s testSeal) Wrap(plain []byte) ([]byte, error) {
	return append([]byte(s.marker), reverse(plain)...), nil
}

func (s testSeal) Unwrap(stored []byte) ([]byte, bool, error) {
	if !bytes.HasPrefix(stored, []byte(s.marker)) {
		return stored, false, nil
	}
	return reverse(stored[len(s.marker):]), true, nil
}

func reverse(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[len(b)-1-i] = b[i]
	}
	return out
}

const accountKey = "PRIVATE-KEY-MATERIAL"

// TestLocalStore_SealedAtRest is the pin that matters: what lands on the node
// does not contain the account key, and the store still reads its own writes.
func TestLocalStore_SealedAtRest(t *testing.T) {
	file := filepath.Join(t.TempDir(), "acme.json")
	seal := testSeal{marker: "SEALED:"}

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

	// A second store over the same file reads it back — the seal is symmetric
	// and the document survives a restart.
	again := NewLocalStore(file, safe.NewPool(t.Context()), seal)
	account, err := again.GetAccount("letsencrypt")
	require.NoError(t, err)
	require.NotNil(t, account)
	assert.Equal(t, []byte(accountKey), account.PrivateKey)
}

// TestLocalStore_SealsAPlaintextStoreOnBoot is the migration. An edge upgrading
// from a plaintext acme.json keeps its certificates — re-ordering them would
// cost a Let's Encrypt rate limit across every host it serves — and the clear
// copy is replaced immediately rather than at the next renewal sixty days out.
func TestLocalStore_SealsAPlaintextStoreOnBoot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "acme.json")
	legacy := map[string]*StoredData{"letsencrypt": {
		Account: &Account{Email: "dev@hanzo.ai", PrivateKey: []byte(accountKey)},
	}}
	blob, err := json.MarshalIndent(legacy, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, blob, 0o600))

	s := NewLocalStore(file, safe.NewPool(t.Context()), testSeal{marker: "SEALED:"})

	account, err := s.GetAccount("letsencrypt")
	require.NoError(t, err)
	require.NotNil(t, account, "the upgrade lost the ACME account")
	assert.Equal(t, []byte(accountKey), account.PrivateKey)

	time.Sleep(200 * time.Millisecond)
	onDisk, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.NotContains(t, string(onDisk), accountKey,
		"the clear copy was still on the node after the first read")
}

// A store that cannot seal writes NOTHING. Falling through to an unsealed write
// is the one failure this path must not have.
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

func (brokenSeal) Wrap([]byte) ([]byte, error) { return nil, assert.AnError }

func (brokenSeal) Unwrap(b []byte) ([]byte, bool, error) { return b, false, nil }

// Plain() is what every deployment ran before sealing existed, and it must stay
// byte-for-byte what it was: an operator reading acme.json on a node still sees
// the document they always saw.
func TestPlain_IsTheDocumentItself(t *testing.T) {
	doc := []byte(`{"letsencrypt":{"Account":null,"Certificates":null}}`)
	wrapped, err := Plain().Wrap(doc)
	require.NoError(t, err)
	assert.Equal(t, doc, wrapped)

	plain, sealed, err := Plain().Unwrap(doc)
	require.NoError(t, err)
	assert.False(t, sealed)
	assert.Equal(t, doc, plain)
}
