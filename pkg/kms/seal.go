package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Seal encrypts the ACME state document at rest.
//
// The document holds the ACME account key and every leaf private key the edge
// has been issued: whoever reads it can impersonate every domain the estate
// serves and can re-issue for the rest. It used to be JSON on a node-local
// hostPath at mode 0600 — which protects it from another UID on that node and
// from nothing else. Sealed, the file is inert: the key that opens it is
// fetched from KMS at startup and exists only in this process's memory.
//
// Envelope, not raw encryption. Each write mints a fresh 256-bit data key,
// encrypts the document under it, and encrypts THAT key under the key from KMS.
// Two consequences worth the twenty extra lines: the long-lived key encrypts
// only 48 bytes per write rather than the whole document, so it is nowhere near
// any usage bound; and recovering one write's data key opens that write and
// nothing else.
//
// AES-256-GCM both times. Authenticated, so a tampered document fails to open
// rather than parsing into something the ACME provider would act on.
type Seal struct{ kek cipher.AEAD }

// envelope is the sealed document as written. It is JSON so that a file which
// is sealed announces it — the alternative is an opaque blob that reads as a
// corrupt store — and so that Unwrap can tell a sealed document from one
// written before sealing was configured without guessing.
type envelope struct {
	Seal int    `json:"seal"` // format version
	Key  string `json:"key"`  // nonce || data key sealed under the KMS key
	Data string `json:"data"` // nonce || document sealed under the data key
}

const sealVersion = 1

// NewSeal builds a seal over a 256-bit key.
func NewSeal(key []byte) (*Seal, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("kms: seal key is %d bytes, want 32", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	kek, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Seal{kek: kek}, nil
}

// SealFrom fetches the sealing key from KMS. The value is 32 bytes as hex or
// base64 — both, because a key generated with `openssl rand -hex 32` and one
// generated with `-base64 32` are the same key and neither spelling should be
// the wrong one.
func SealFrom(ctx context.Context, c *Client, name string) (*Seal, error) {
	raw, err := c.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	key, err := decodeKey(raw)
	if err != nil {
		return nil, fmt.Errorf("kms: %s: %w", name, err)
	}
	return NewSeal(key)
}

func decodeKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if b, err := hex.DecodeString(raw); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(raw); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.RawURLEncoding.DecodeString(raw); err == nil && len(b) == 32 {
		return b, nil
	}
	return nil, errors.New("not a 32-byte key in hex or base64")
}

// Wrap seals plain for storage.
func (s *Seal) Wrap(plain []byte) ([]byte, error) {
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return nil, err
	}
	data, err := newAEAD(dek)
	if err != nil {
		return nil, err
	}
	sealedDoc, err := seal(data, plain)
	if err != nil {
		return nil, err
	}
	sealedKey, err := seal(s.kek, dek)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(envelope{
		Seal: sealVersion,
		Key:  base64.StdEncoding.EncodeToString(sealedKey),
		Data: base64.StdEncoding.EncodeToString(sealedDoc),
	}, "", "  ")
}

// Unwrap returns the plaintext of a stored document.
//
// A document written before sealing was configured is returned unchanged with
// sealed=false. That is the whole migration: an edge that has been running with
// a plaintext store keeps its certificates across the upgrade instead of
// re-ordering every one of them against a rate limit. The caller rewrites it
// under seal immediately, so the clear copy survives one boot and no longer.
func (s *Seal) Unwrap(b []byte) (plain []byte, sealed bool, err error) {
	env, ok := parseEnvelope(b)
	if !ok {
		return b, false, nil
	}
	if env.Seal != sealVersion {
		return nil, true, fmt.Errorf("kms: sealed document version %d, want %d", env.Seal, sealVersion)
	}
	sealedKey, err := base64.StdEncoding.DecodeString(env.Key)
	if err != nil {
		return nil, true, fmt.Errorf("kms: sealed key: %w", err)
	}
	dek, err := open(s.kek, sealedKey)
	if err != nil {
		return nil, true, fmt.Errorf("kms: unwrap data key: %w", err)
	}
	data, err := newAEAD(dek)
	if err != nil {
		return nil, true, err
	}
	sealedDoc, err := base64.StdEncoding.DecodeString(env.Data)
	if err != nil {
		return nil, true, fmt.Errorf("kms: sealed document: %w", err)
	}
	doc, err := open(data, sealedDoc)
	if err != nil {
		return nil, true, fmt.Errorf("kms: unseal document: %w", err)
	}
	return doc, true, nil
}

// parseEnvelope reports whether b is a sealed document. A plaintext ACME store
// is a JSON OBJECT of resolvers, so it decodes into this shape with a zero
// version — which is what distinguishes the two, not the presence of the keys.
func parseEnvelope(b []byte) (envelope, bool) {
	var env envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return envelope{}, false
	}
	return env, env.Seal != 0
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// seal returns nonce || ciphertext. The nonce is random per call and carried
// with the ciphertext, so no caller has to keep a counter and no two writes can
// reuse one.
func seal(a cipher.AEAD, plain []byte) ([]byte, error) {
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, plain, nil), nil
}

func open(a cipher.AEAD, b []byte) ([]byte, error) {
	if len(b) < a.NonceSize() {
		return nil, errors.New("ciphertext shorter than its nonce")
	}
	return a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], nil)
}
