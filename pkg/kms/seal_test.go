package kms

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func key(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func newSeal(t *testing.T, k []byte) *Seal {
	t.Helper()
	s, err := NewSeal(k, false)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// store is the name a document is written for. Everything below writes under
// this one and reads back under it, except where the point is that it does not.
const store = "hanzo/ingress-acme"

// acmeLike is the shape of the document being protected: a resolver map whose
// leaves hold PEM private keys. Using the real shape is the point — the test
// then also proves no key material survives into the sealed bytes.
const acmeLike = `{"letsencrypt":{"Account":{"Email":"dev@hanzo.ai",` +
	`"PrivateKey":"MIIEowIBAAKCAQEAvSECRETACCOUNTKEY"},` +
	`"Certificates":[{"domain":{"main":"api.hanzo.ai"},"key":"LS0tLSSECRETLEAFKEY"}]}}`

func TestSeal_RoundTrip(t *testing.T) {
	s := newSeal(t, key(t))

	sealed, err := s.Wrap(store, 7, []byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	plain, count, wasSealed, err := s.Unwrap(store, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !wasSealed {
		t.Error("Unwrap reported a sealed document as unsealed")
	}
	if count != 7 {
		t.Errorf("count = %d, want 7", count)
	}
	if string(plain) != acmeLike {
		t.Errorf("round trip changed the document:\n have %s\n want %s", plain, acmeLike)
	}
}

// The whole point: what lands on the node must not contain the keys.
func TestSeal_CiphertextCarriesNoKeyMaterial(t *testing.T) {
	s := newSeal(t, key(t))
	sealed, err := s.Wrap(store, 1, []byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SECRETACCOUNTKEY", "SECRETLEAFKEY", "api.hanzo.ai", "dev@hanzo.ai"} {
		if bytes.Contains(sealed, []byte(secret)) {
			t.Errorf("sealed document contains %q in the clear", secret)
		}
	}
}

// A fresh data key and nonce per write: the same document sealed twice is two
// different ciphertexts, so nothing can be learned by comparing writes.
func TestSeal_EveryWriteIsDistinct(t *testing.T) {
	s := newSeal(t, key(t))
	a, err := s.Wrap(store, 1, []byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Wrap(store, 1, []byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Error("two writes of the same document produced the same bytes")
	}
}

// A store that is not sealed is refused. That is the default and it is the
// answer that matters: this process was pointed at a store it did not write,
// and writing this edge's account key over it is not a recovery.
func TestSeal_RefusesAnUnsealedStore(t *testing.T) {
	s := newSeal(t, key(t))
	_, _, sealed, err := s.Unwrap(store, []byte(acmeLike))
	if !errors.Is(err, ErrUnsealed) {
		t.Fatalf("Unwrap(plaintext) = %v, want ErrUnsealed", err)
	}
	if sealed {
		t.Error("Unwrap reported an unsealed document as sealed")
	}
}

// Adopting is the operator saying "this store is mine, take it under seal".
// It opens once, reports the document unsealed so the caller writes it back,
// and every write it makes from then on is sealed.
func TestSeal_AdoptsAnUnsealedStoreOnce(t *testing.T) {
	k := key(t)
	s, err := NewSeal(k, true)
	if err != nil {
		t.Fatal(err)
	}
	plain, count, sealed, err := s.Unwrap(store, []byte(acmeLike))
	if err != nil {
		t.Fatalf("adopting seal refused a plaintext store: %v", err)
	}
	if sealed || count != 0 {
		t.Errorf("adopted document reported sealed=%v count=%d, want false/0", sealed, count)
	}
	if string(plain) != acmeLike {
		t.Error("adoption changed the document")
	}

	// What it writes back is sealed, and the refusing seal over the same key
	// reads it — adoption is a property of the read, never of the write.
	written, err := s.Wrap(store, 1, plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, wasSealed, err := newSeal(t, k).Unwrap(store, written); err != nil || !wasSealed {
		t.Errorf("an adopting seal wrote something the refusing seal cannot read: sealed=%v err=%v", wasSealed, err)
	}
}

// Adoption covers ONE store. A second, different unsealed document is not the
// store the operator was looking at, and it is refused — otherwise a seal left
// adopting takes whatever plaintext it is handed for as long as the process
// runs.
func TestSeal_AdoptsOneStoreOnly(t *testing.T) {
	s, err := NewSeal(key(t), true)
	if err != nil {
		t.Fatal(err)
	}
	first := []byte(acmeLike)
	other := []byte(`{"letsencrypt":{"Account":{"Email":"someone@example.invalid"}}}`)

	if _, _, _, err := s.Unwrap(store, first); err != nil {
		t.Fatalf("the first unsealed store was refused: %v", err)
	}
	// The same document again: a store is read more than once before the write
	// that seals it lands, and a replica that cannot write reads it every poll.
	if _, _, _, err := s.Unwrap(store, first); err != nil {
		t.Errorf("re-reading the adopted store was refused: %v", err)
	}
	if _, _, _, err := s.Unwrap(store, other); !errors.Is(err, ErrUnsealed) {
		t.Fatalf("a second, different unsealed store was adopted: err=%v", err)
	}
}

// Adoption ends when a sealed write lands. The residual it closes: an adopted
// plaintext keeps the same digest as the pre-seal snapshot, so a rollback that
// replants that snapshot would be re-admitted as long as the process kept
// adopting. Once the store is sealed, Persisted ends it, and the replanted
// snapshot is refused.
func TestSeal_AdoptionEndsAfterASealedWriteLands(t *testing.T) {
	s, err := NewSeal(key(t), true)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := []byte(acmeLike)

	// Adopt the store and write it back under seal, exactly as a store does.
	plain, _, sealed, err := s.Unwrap(store, snapshot)
	if err != nil || sealed {
		t.Fatalf("adopt: plain err=%v sealed=%v", err, sealed)
	}
	written, err := s.Wrap(store, 1, plain)
	if err != nil {
		t.Fatal(err)
	}
	s.Persisted() // the store confirms the sealed write reached it

	// The sealed store still opens — reads are unaffected.
	if _, _, wasSealed, err := s.Unwrap(store, written); err != nil || !wasSealed {
		t.Errorf("sealed read after Persisted: sealed=%v err=%v", wasSealed, err)
	}
	// The pre-seal snapshot, replanted, is now refused — even though it is the
	// very bytes adoption admitted a moment ago.
	if _, _, _, err := s.Unwrap(store, snapshot); !errors.Is(err, ErrUnsealed) {
		t.Fatalf("a replanted pre-seal snapshot was admitted after the store was sealed: %v", err)
	}
}

// A document belongs to the store it was written for. Moving one between
// stores does not open it, because the store's name is what both layers were
// sealed with and not merely what the JSON says.
func TestSeal_RefusesADocumentFromAnotherStore(t *testing.T) {
	s := newSeal(t, key(t))
	sealed, err := s.Wrap("hanzo/ingress-acme", 3, []byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Unwrap("lux-system/ingress-acme", sealed); err == nil {
		t.Fatal("a document sealed for one store opened as another")
	}
	if _, _, _, err := s.Unwrap("/data/acme.json", sealed); err == nil {
		t.Fatal("a document sealed for a Secret opened as a file")
	}
}

// Two names that concatenate to the same bytes must not collide, which is what
// the length prefixes in the bound data are for.
func TestSeal_StoreNamesDoNotRunTogether(t *testing.T) {
	s := newSeal(t, key(t))
	sealed, err := s.Wrap("ab", 1, []byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Unwrap("a", sealed); err == nil {
		t.Fatal(`a document sealed for "ab" opened as "a"`)
	}
}

// The count is sealed with the document, so an older write cannot be presented
// as a newer one by editing the envelope.
func TestSeal_CountCannotBeEdited(t *testing.T) {
	s := newSeal(t, key(t))
	sealed, err := s.Wrap(store, 4, []byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	var env envelope
	if err := json.Unmarshal(sealed, &env); err != nil {
		t.Fatal(err)
	}
	env.Count = 99
	edited, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Unwrap(store, edited); err == nil {
		t.Fatal("an envelope with an edited count opened")
	}
}

// A retired key names itself, so a document sealed under it reads as the wrong
// key rather than as a corrupt store.
func TestSeal_NamesTheKeyItWasSealedUnder(t *testing.T) {
	old := newSeal(t, key(t))
	sealed, err := old.Wrap(store, 1, []byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = newSeal(t, key(t)).Unwrap(store, sealed)
	if err == nil {
		t.Fatal("a document opened under a key that did not seal it")
	}
	if !strings.Contains(err.Error(), old.ID()) {
		t.Errorf("error %q does not name the key that sealed the document (%s)", err, old.ID())
	}
}

// Zero is the count of a document never written under seal, so it is not a
// count a write may claim.
func TestSeal_WriteIsCountedFromOne(t *testing.T) {
	if _, err := newSeal(t, key(t)).Wrap(store, 0, []byte(acmeLike)); err == nil {
		t.Fatal("Wrap accepted write 0")
	}
}

// A tampered document does not parse into something the ACME provider would
// act on — it fails to open.
func TestSeal_RefusesTamperedCiphertext(t *testing.T) {
	s := newSeal(t, key(t))
	sealed, err := s.Wrap(store, 1, []byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	var env envelope
	if err := json.Unmarshal(sealed, &env); err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(env.Data)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0x01
	env.Data = base64.StdEncoding.EncodeToString(raw)
	tampered, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.Unwrap(store, tampered); err == nil {
		t.Fatal("a tampered document opened")
	}
}

// The key arrives from KMS as whatever an operator's `openssl rand` produced.
func TestDecodeKey_AcceptsBothSpellings(t *testing.T) {
	k := key(t)
	for name, raw := range map[string]string{
		"hex":       hex.EncodeToString(k),
		"base64":    base64.StdEncoding.EncodeToString(k),
		"rawurl":    base64.RawURLEncoding.EncodeToString(k),
		"hex+space": " " + hex.EncodeToString(k) + "\n",
	} {
		got, err := decodeKey(raw)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !bytes.Equal(got, k) {
			t.Errorf("%s: decoded to a different key", name)
		}
	}
	if _, err := decodeKey("not-a-key"); err == nil {
		t.Error("decodeKey accepted a value that is not a key")
	}
	if _, err := decodeKey(hex.EncodeToString(k[:16])); err == nil {
		t.Error("decodeKey accepted a 16-byte key")
	}
}

func TestNewSeal_RefusesAKeyOfTheWrongLength(t *testing.T) {
	if _, err := NewSeal(make([]byte, 16), false); err == nil {
		t.Error("NewSeal accepted a 16-byte key")
	}
}
