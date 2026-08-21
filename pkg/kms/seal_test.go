package kms

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
	s, err := NewSeal(k)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// acmeLike is the shape of the document being protected: a resolver map whose
// leaves hold PEM private keys. Using the real shape is the point — the test
// then also proves no key material survives into the sealed bytes.
const acmeLike = `{"letsencrypt":{"Account":{"Email":"dev@hanzo.ai",` +
	`"PrivateKey":"MIIEowIBAAKCAQEAvSECRETACCOUNTKEY"},` +
	`"Certificates":[{"domain":{"main":"api.hanzo.ai"},"key":"LS0tLSSECRETLEAFKEY"}]}}`

func TestSeal_RoundTrip(t *testing.T) {
	s := newSeal(t, key(t))

	sealed, err := s.Wrap([]byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	plain, wasSealed, err := s.Unwrap(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !wasSealed {
		t.Error("Unwrap reported a sealed document as unsealed")
	}
	if string(plain) != acmeLike {
		t.Errorf("round trip changed the document:\n have %s\n want %s", plain, acmeLike)
	}
}

// The whole point: what lands on the node must not contain the keys.
func TestSeal_CiphertextCarriesNoKeyMaterial(t *testing.T) {
	s := newSeal(t, key(t))
	sealed, err := s.Wrap([]byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SECRETACCOUNTKEY", "SECRETLEAFKEY", "api.hanzo.ai", "dev@hanzo.ai"} {
		if bytes.Contains(sealed, []byte(secret)) {
			t.Errorf("sealed document contains %q in the clear", secret)
		}
	}
}

// A fresh data key and nonce per write: the same document sealed twice must not
// produce the same bytes, or an observer of the node learns when it changed.
func TestSeal_WritesAreDistinct(t *testing.T) {
	s := newSeal(t, key(t))
	a, err := s.Wrap([]byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Wrap([]byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Error("two writes of one document produced identical bytes")
	}
}

// GCM is authenticated, so a document an attacker edited on the node fails to
// open rather than parsing into something the ACME provider would act on.
func TestSeal_TamperIsRefused(t *testing.T) {
	s := newSeal(t, key(t))
	sealed, err := s.Wrap([]byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}

	for _, field := range []string{"data", "key"} {
		var env envelope
		if err := json.Unmarshal(sealed, &env); err != nil {
			t.Fatal(err)
		}
		target := &env.Data
		if field == "key" {
			target = &env.Key
		}
		raw, err := base64.StdEncoding.DecodeString(*target)
		if err != nil {
			t.Fatal(err)
		}
		raw[len(raw)-1] ^= 0x01
		*target = base64.StdEncoding.EncodeToString(raw)

		edited, err := json.Marshal(env)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.Unwrap(edited); err == nil {
			t.Errorf("a tampered %q field opened without error", field)
		}
	}
}

func TestSeal_WrongKeyIsRefused(t *testing.T) {
	sealed, err := newSeal(t, key(t)).Wrap([]byte(acmeLike))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := newSeal(t, key(t)).Unwrap(sealed); err == nil {
		t.Error("a document opened under a key that did not seal it")
	}
}

// The migration: an edge upgrading from a plaintext store keeps its
// certificates. Returning them with sealed=false is what tells the store to
// rewrite the file, so the clear copy survives one boot and no longer.
func TestSeal_PlaintextDocumentIsReadableOnce(t *testing.T) {
	plain, sealed, err := newSeal(t, key(t)).Unwrap([]byte(acmeLike))
	if err != nil {
		t.Fatalf("a plaintext store must stay readable across the upgrade: %v", err)
	}
	if sealed {
		t.Error("a plaintext document was reported as sealed; it would never be rewritten")
	}
	if string(plain) != acmeLike {
		t.Error("plaintext document was altered")
	}
}

func TestSeal_RejectsShortKey(t *testing.T) {
	for _, n := range []int{0, 16, 31, 33} {
		if _, err := NewSeal(make([]byte, n)); err == nil {
			t.Errorf("NewSeal accepted a %d-byte key", n)
		}
	}
}

func TestDecodeKey(t *testing.T) {
	k := key(t)
	for name, spelling := range map[string]string{
		"hex":         hex.EncodeToString(k),
		"base64":      base64.StdEncoding.EncodeToString(k),
		"base64url":   base64.RawURLEncoding.EncodeToString(k),
		"hex, padded": "  " + hex.EncodeToString(k) + "\n",
	} {
		got, err := decodeKey(spelling)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !bytes.Equal(got, k) {
			t.Errorf("%s: decoded to a different key", name)
		}
	}
	for _, bad := range []string{"", "not-a-key", hex.EncodeToString(make([]byte, 16))} {
		if _, err := decodeKey(bad); err == nil {
			t.Errorf("decodeKey(%q) accepted a value that is not a 256-bit key", bad)
		}
	}
}
