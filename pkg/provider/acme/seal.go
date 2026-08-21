package acme

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/hanzoai/ingress/pkg/observability/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"

	"github.com/hanzoai/ingress/pkg/observability/logs"
)

// Seal is how the ACME state document becomes bytes at rest.
//
// It is an interface here, and its implementation lives in pkg/kms, because
// this package's job is what the document MEANS — accounts, certificates,
// resolvers — and not what a key is. Inverting it that way also keeps the ACME
// provider's import graph free of anything that talks to a network.
//
// A store is named, and every write is counted. Both travel into the seal
// because both are what the bytes are bound to: a document opens for the store
// it was written for, at the write it was made at, and nowhere else.
type Seal interface {
	// Wrap returns the bytes to persist for the store called name, as its
	// count-th write. Counting starts at one.
	Wrap(name string, count uint64, plain []byte) ([]byte, error)

	// Unwrap returns the document the store read, the write it was made at,
	// and whether the bytes were under seal. A configured seal returns an
	// error for bytes that carry no envelope unless the operator has adopted
	// the store; then it returns them with sealed false, for the caller to
	// write back under seal.
	Unwrap(name string, stored []byte) (plain []byte, count uint64, sealed bool, err error)
}

// plain is the seal of a deployment that has configured none: the document is
// written as it always was. It exists so that no store has to ask whether it
// has a seal — a nil check that one of them would eventually forget, on the
// path whose entire job is to not write private keys in the clear.
type plain struct{}

// Plain returns the identity seal — JSON at rest, which is what this edge did
// everywhere before KMS sealing existed.
func Plain() Seal { return plain{} }

func (plain) Wrap(_ string, _ uint64, doc []byte) ([]byte, error) { return doc, nil }

func (plain) Unwrap(_ string, stored []byte) ([]byte, uint64, bool, error) {
	return stored, 0, true, nil
}

// unsealed counts the reads that did not open. It is a counter because the
// interesting shape is the rate: one at boot is a store this edge was not
// pointed at, a steady trickle is a store something else is writing.
var unsealed = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: metrics.MetricNamePrefix + "acme_unseal_failures_total",
	Help: "Reads of the ACME state document that could not be opened.",
}, []string{"store"})

func init() { metrics.Register(unsealed) }

// counter is a store's view of how many writes its document has had.
//
// It only moves forward. A document that reports a write BELOW the one this
// replica has already read is not the current document — it is an earlier copy
// of it — and the store keeps what it has rather than stepping back to it.
type counter struct {
	mu sync.Mutex
	n  uint64
}

// read records a document at write n, and refuses one that is behind.
//
// Zero is not a write — it is a document that carries no count, which is what
// an unsealed store is and all an unsealed store can be. It makes no claim
// about being current, so it neither advances the count nor steps it back. A
// sealed document can never arrive at zero: Wrap counts from one and Unwrap
// refuses an envelope claiming otherwise.
func (c *counter) read(n uint64) error {
	if n == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if n < c.n {
		return fmt.Errorf("acme store: document is at write %d, this replica has read %d", n, c.n)
	}
	c.n = n
	return nil
}

// next reserves the write a store is about to make. It advances whether or not
// that write lands: the numbering has to increase, it does not have to be
// dense, and one call is one thing to remember instead of two.
func (c *counter) next() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return c.n
}

// encodeStored renders the state document for storage as the count-th write of
// the store called name.
//
// Indented, because a store a human can look at is worth two spaces, and
// because the sealed envelope carries its ciphertext base64 either way.
func encodeStored(seal Seal, name string, count uint64, state map[string]*StoredData) ([]byte, error) {
	doc, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("acme store: encode: %w", err)
	}
	sealed, err := seal.Wrap(name, count, doc)
	if err != nil {
		return nil, fmt.Errorf("acme store: seal: %w", err)
	}
	return sealed, nil
}

// decodeStored parses the state document, treating empty as empty rather than
// as an error — a store that has never been written is the normal first boot.
//
// A read that does not open is an error and is counted. Falling through to an
// empty document would look like a first boot and would order the estate again
// on top of a store that is already there.
func decodeStored(seal Seal, name string, stored []byte) (map[string]*StoredData, uint64, bool, error) {
	out := map[string]*StoredData{}
	if len(stored) == 0 {
		return out, 0, true, nil
	}
	doc, count, sealed, err := seal.Unwrap(name, stored)
	if err != nil {
		unsealed.WithLabelValues(name).Inc()
		log.Error().Err(err).Str(logs.ProviderName, "acme").Str("store", name).
			Msg("ACME state could not be opened")
		return nil, 0, sealed, fmt.Errorf("acme store: unseal: %w", err)
	}
	if err := json.Unmarshal(doc, &out); err != nil {
		unsealed.WithLabelValues(name).Inc()
		log.Error().Err(err).Str(logs.ProviderName, "acme").Str("store", name).
			Msg("ACME state could not be read")
		return nil, 0, sealed, fmt.Errorf("acme store: decode: %w", err)
	}
	return out, count, sealed, nil
}
