package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Seal encrypts the ACME state document at rest.
//
// The document holds the ACME account key and every leaf private key the edge
// has been issued: whoever reads it can serve every domain the estate serves
// and can re-issue for the rest. Sealed, the file is inert — the key that opens
// it is fetched from KMS at startup and exists only in this process's memory.
//
// Envelope, not raw encryption. Each write mints a fresh 256-bit data key,
// encrypts the document under it, and encrypts THAT key under the key from KMS.
// Two consequences worth the twenty extra lines: the long-lived key encrypts
// only 48 bytes per write rather than the whole document, so it is nowhere near
// any usage bound; and recovering one write's data key opens that write and
// nothing else.
//
// AES-256-GCM both times, and both layers are bound to the same four facts:
// the format version, WHICH store the bytes belong to, WHICH key sealed them,
// and WHICH write they are. Those facts travel in the envelope and in the
// additional data of both seals, so the bytes open only where they were
// written, under the key they were written with, at the write they were made
// at. Moving an envelope between stores, re-pointing it at another key, or
// editing its count all fail to open rather than opening into something the
// ACME provider would act on.
type Seal struct {
	kek   cipher.AEAD
	id    string
	adopt bool
}

// envelope is the sealed document as written. It is JSON so that a file which
// is sealed announces it, and so a reader can tell a sealed document from one
// written before sealing was configured without guessing.
type envelope struct {
	Seal  int    `json:"seal"`  // format version
	ID    string `json:"id"`    // fingerprint of the key this was sealed under
	Count uint64 `json:"count"` // which write this is, per store
	Key   string `json:"key"`   // nonce || data key sealed under the KMS key
	Data  string `json:"data"`  // nonce || document sealed under the data key
}

const version = 1

// ErrUnsealed reports bytes that carry no envelope. It is a distinct error
// because it is the one an operator can answer: a store written before sealing
// was configured is taken under seal by adopting it once.
var ErrUnsealed = errors.New("kms: the stored document is not sealed")

// NewSeal builds a seal over a 256-bit key.
//
// adopt says what to do with a store that is not sealed. False — the default
// everywhere — reads it as an error: the store this process was pointed at is
// not the store this key wrote, and continuing would mean writing this edge's
// account key over whatever is there. True opens it once and hands it back for
// the caller to write under seal, which is how a store that predates sealing
// keeps its certificates instead of re-ordering every one of them.
func NewSeal(key []byte, adopt bool) (*Seal, error) {
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
	return &Seal{kek: kek, id: fingerprint(key), adopt: adopt}, nil
}

// fingerprint names a key without revealing it: half a SHA-256 over a fixed
// label and the key. It goes in the envelope so a document sealed under a
// retired key says so, instead of reading as corrupt.
func fingerprint(key []byte) string {
	sum := sha256.Sum256(append([]byte("hanzo ingress acme seal\x00"), key...))
	return hex.EncodeToString(sum[:8])
}

// SealFrom fetches the sealing key from KMS. The value is 32 bytes as hex or
// base64 — both, because a key generated with `openssl rand -hex 32` and one
// generated with `-base64 32` are the same key and neither spelling should be
// the wrong one.
func SealFrom(ctx context.Context, c *Client, name string, adopt bool) (*Seal, error) {
	raw, err := c.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	key, err := decodeKey(raw)
	if err != nil {
		return nil, fmt.Errorf("kms: %s: %w", name, err)
	}
	return NewSeal(key, adopt)
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

// ID is the fingerprint of the key this seal holds. A store logs it so an
// operator can see which key an edge is running under.
func (s *Seal) ID() string { return s.id }

// Wrap seals plain as the count-th write of the store called name.
//
// Counting starts at one. Zero is the count of a document that was never
// written under seal, so it is not a count a write may claim.
func (s *Seal) Wrap(name string, count uint64, plain []byte) ([]byte, error) {
	if count == 0 {
		return nil, errors.New("kms: a sealed write is counted from one")
	}
	aad := bind(version, name, s.id, count)

	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return nil, err
	}
	data, err := newAEAD(dek)
	if err != nil {
		return nil, err
	}
	sealedDoc, err := seal(data, plain, aad)
	if err != nil {
		return nil, err
	}
	sealedKey, err := seal(s.kek, dek, aad)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(envelope{
		Seal:  version,
		ID:    s.id,
		Count: count,
		Key:   base64.StdEncoding.EncodeToString(sealedKey),
		Data:  base64.StdEncoding.EncodeToString(sealedDoc),
	}, "", "  ")
}

// Unwrap returns the document stored for the store called name, the write it
// was made at, and whether it was under seal.
//
// The envelope's own version, store name, key id and count are the additional
// data both layers were sealed with, so every one of them is verified by the
// decryption rather than trusted from the JSON. An envelope written for another
// store, under another key, or edited to claim another count does not open.
//
// Bytes that carry no envelope are ErrUnsealed, unless this seal was built to
// adopt one: then they come back as they are, with sealed false, and the caller
// writes them under seal.
func (s *Seal) Unwrap(name string, stored []byte) (plain []byte, count uint64, sealed bool, err error) {
	env, ok := parse(stored)
	if !ok {
		if !s.adopt {
			return nil, 0, false, ErrUnsealed
		}
		return stored, 0, false, nil
	}
	if env.Seal != version {
		return nil, 0, true, fmt.Errorf("kms: sealed document version %d, want %d", env.Seal, version)
	}
	if env.ID != s.id {
		return nil, 0, true, fmt.Errorf("kms: document sealed under key %s, this edge holds %s", env.ID, s.id)
	}
	if env.Count == 0 {
		return nil, 0, true, errors.New("kms: sealed document claims write 0")
	}
	aad := bind(env.Seal, name, env.ID, env.Count)

	sealedKey, err := base64.StdEncoding.DecodeString(env.Key)
	if err != nil {
		return nil, 0, true, fmt.Errorf("kms: sealed key: %w", err)
	}
	dek, err := open(s.kek, sealedKey, aad)
	if err != nil {
		return nil, 0, true, fmt.Errorf("kms: unwrap data key: %w", err)
	}
	data, err := newAEAD(dek)
	if err != nil {
		return nil, 0, true, err
	}
	sealedDoc, err := base64.StdEncoding.DecodeString(env.Data)
	if err != nil {
		return nil, 0, true, fmt.Errorf("kms: sealed document: %w", err)
	}
	doc, err := open(data, sealedDoc, aad)
	if err != nil {
		return nil, 0, true, fmt.Errorf("kms: unseal document: %w", err)
	}
	return doc, env.Count, true, nil
}

// bind renders the facts the ciphertext is tied to. Each variable-length field
// is written after its length, so no combination of a store name and a key id
// can produce the bytes of another combination.
func bind(version int, name, id string, count uint64) []byte {
	b := binary.BigEndian.AppendUint64(nil, uint64(version))
	b = field(b, name)
	b = field(b, id)
	return binary.BigEndian.AppendUint64(b, count)
}

func field(b []byte, s string) []byte {
	b = binary.BigEndian.AppendUint64(b, uint64(len(s)))
	return append(b, s...)
}

// parse reports whether b is a sealed document. A plaintext ACME store is a
// JSON OBJECT of resolvers, so it decodes into this shape with a zero version —
// which is what distinguishes the two, not the presence of the keys.
func parse(b []byte) (envelope, bool) {
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
func seal(a cipher.AEAD, plain, aad []byte) ([]byte, error) {
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, plain, aad), nil
}

func open(a cipher.AEAD, b, aad []byte) ([]byte, error) {
	if len(b) < a.NonceSize() {
		return nil, errors.New("ciphertext shorter than its nonce")
	}
	return a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], aad)
}
