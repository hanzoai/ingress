package acme

import (
	"encoding/json"
	"fmt"
)

// Seal is how the ACME state document becomes bytes at rest.
//
// It is an interface here, and its implementation lives in pkg/kms, because
// this package's job is what the document MEANS — accounts, certificates,
// resolvers — and not what a key is. Inverting it that way also keeps the ACME
// provider's import graph free of anything that talks to a network.
type Seal interface {
	// Wrap returns the bytes to persist.
	Wrap(plain []byte) ([]byte, error)
	// Unwrap returns the document a store read. A document written before
	// sealing was configured comes back unchanged with sealed=false, so a
	// store can keep its certificates across the upgrade and rewrite them
	// under seal rather than re-ordering every one of them.
	Unwrap(stored []byte) (plain []byte, sealed bool, err error)
}

// plain is the seal of a deployment that has configured none: the document is
// written as it always was. It exists so that no store has to ask whether it
// has a seal — a nil check that one of them would eventually forget, on the
// path whose entire job is to not write private keys in the clear.
type plain struct{}

// Plain returns the identity seal — JSON at rest, which is what this edge did
// everywhere before KMS sealing existed.
func Plain() Seal { return plain{} }

func (plain) Wrap(doc []byte) ([]byte, error) { return doc, nil }

func (plain) Unwrap(stored []byte) ([]byte, bool, error) { return stored, false, nil }

// encodeStored renders the state document for storage under seal.
//
// Indented, because a store that a human can look at is worth two spaces, and
// because the sealed envelope carries its ciphertext base64 either way.
func encodeStored(seal Seal, state map[string]*StoredData) ([]byte, error) {
	plain, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("acme store: encode: %w", err)
	}
	sealed, err := seal.Wrap(plain)
	if err != nil {
		return nil, fmt.Errorf("acme store: seal: %w", err)
	}
	return sealed, nil
}

// decodeStored parses the state document, treating empty as empty rather than
// as an error — a store that has never been written is the normal first boot.
//
// The second return says whether the bytes were sealed at rest. False on a
// non-empty document under a real seal means this boot found a plaintext store
// and should rewrite it.
func decodeStored(seal Seal, stored []byte) (map[string]*StoredData, bool, error) {
	out := map[string]*StoredData{}
	if len(stored) == 0 {
		return out, true, nil
	}
	plain, sealed, err := seal.Unwrap(stored)
	if err != nil {
		return nil, sealed, fmt.Errorf("acme store: unseal: %w", err)
	}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, sealed, fmt.Errorf("acme store: decode: %w", err)
	}
	return out, sealed, nil
}
