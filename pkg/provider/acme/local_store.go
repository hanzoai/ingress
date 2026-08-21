package acme

import (
	"context"
	"io"
	"maps"
	"os"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/hanzoai/ingress/pkg/observability/logs"
	"github.com/hanzoai/ingress/pkg/safe"
)

var _ Store = (*LocalStore)(nil)

// LocalStore Stores implementation for local file.
type LocalStore struct {
	saveDataChan chan map[string]*StoredData
	filename     string
	seal         Seal
	count        counter

	lock       sync.RWMutex
	storedData map[string]*StoredData
}

// NewLocalStore initializes a new LocalStore with a file name.
//
// The seal is how the document reaches the disk. Pass acme.Plain() to write it
// as plain JSON — mode 0600 and nothing else, which is what protects a node's
// ACME account key and every leaf key it holds from exactly one attacker: a
// different UID on the same node.
func NewLocalStore(filename string, routinesPool *safe.Pool, seal Seal) *LocalStore {
	store := &LocalStore{filename: filename, seal: seal, saveDataChan: make(chan map[string]*StoredData)}
	store.listenSaveAction(routinesPool)
	return store
}

// GetAccount returns ACME Account.
func (s *LocalStore) GetAccount(resolverName string) (*Account, error) {
	storedData, err := s.get(resolverName)
	if err != nil {
		return nil, err
	}

	return storedData.Account, nil
}

// SaveAccount stores ACME Account.
func (s *LocalStore) SaveAccount(resolverName string, account *Account) error {
	storedData, err := s.get(resolverName)
	if err != nil {
		return err
	}

	storedData.Account = account
	s.save(resolverName, storedData)

	return nil
}

// GetCertificates returns ACME Certificates list.
func (s *LocalStore) GetCertificates(resolverName string) ([]*CertAndStore, error) {
	storedData, err := s.get(resolverName)
	if err != nil {
		return nil, err
	}

	return storedData.Certificates, nil
}

// SaveCertificates stores ACME Certificates list.
func (s *LocalStore) SaveCertificates(resolverName string, certificates []*CertAndStore) error {
	storedData, err := s.get(resolverName)
	if err != nil {
		return err
	}

	storedData.Certificates = certificates
	s.save(resolverName, storedData)

	return nil
}

func (s *LocalStore) save(resolverName string, storedData *StoredData) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.storedData[resolverName] = storedData

	// we cannot pass s.storedData directly, map is reference type and as result
	// we can face with race condition, so we need to work with objects copy
	s.saveDataChan <- s.unSafeCopyOfStoredData()
}

func (s *LocalStore) get(resolverName string) (*StoredData, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// storedData is published only once the file has been read, so a read that
	// refuses stays refused. Publishing an empty map first would leave the next
	// call looking at a store that appears new, and an ACME provider answers a
	// new store by ordering the estate again over what is already there.
	if s.storedData == nil {
		loaded := map[string]*StoredData{}

		hasData, err := CheckFile(s.filename)
		if err != nil {
			return nil, err
		}

		if hasData {
			logger := log.With().Str(logs.ProviderName, "acme").Logger()

			f, err := os.Open(s.filename)
			if err != nil {
				return nil, err
			}
			defer f.Close()

			file, err := io.ReadAll(f)
			if err != nil {
				return nil, err
			}

			stored, count, sealed, err := decodeStored(s.seal, s.filename, file)
			if err != nil {
				return nil, err
			}
			if err := s.count.read(count); err != nil {
				return nil, err
			}
			loaded = stored
			if !sealed {
				// The operator adopted a store written before sealing was
				// configured. Write it back under seal now rather than at the
				// next renewal, which is sixty days away.
				logger.Warn().Str("file", s.filename).
					Msg("ACME state adopted unsealed; writing it back under seal")
				s.saveDataChan <- maps.Clone(loaded)
			}

			// Delete all certificates with no value
			var certificates []*CertAndStore
			for _, storedData := range loaded {
				for _, certificate := range storedData.Certificates {
					if len(certificate.Certificate.Certificate) == 0 || len(certificate.Key) == 0 {
						logger.Debug().Msgf("Deleting empty certificate %v for %v", certificate, certificate.Domain.ToStrArray())
						continue
					}
					certificates = append(certificates, certificate)
				}
				if len(certificates) < len(storedData.Certificates) {
					storedData.Certificates = certificates

					// we cannot pass loaded directly, map is reference type and as
					// result we can face with race condition, so we need to work
					// with objects copy
					s.saveDataChan <- maps.Clone(loaded)
				}
			}
		}
		s.storedData = loaded
	}

	if s.storedData[resolverName] == nil {
		s.storedData[resolverName] = &StoredData{}
	}
	return s.storedData[resolverName], nil
}

// listenSaveAction listens to a chan to store ACME data in json format into `LocalStore.filename`.
func (s *LocalStore) listenSaveAction(routinesPool *safe.Pool) {
	routinesPool.GoCtx(func(ctx context.Context) {
		logger := log.With().Str(logs.ProviderName, "acme").Logger()
		for {
			select {
			case <-ctx.Done():
				return

			case object := <-s.saveDataChan:
				select {
				case <-ctx.Done():
					// Stop handling events because Ingress is shutting down.
					return
				default:
				}

				want := s.count.next()
				data, err := encodeStored(s.seal, s.filename, want, object)
				if err != nil {
					// Never fall through to writing the document unsealed:
					// the write that follows an encode failure is the one
					// that would put private keys on the disk in the clear.
					logger.Error().Err(err).Msg("ACME state not written")
					continue
				}

				if err := os.WriteFile(s.filename, data, 0o600); err != nil {
					logger.Error().Err(err).Send()
					continue
				}
				s.count.wrote(want)
				s.seal.Persisted()
			}
		}
	})
}

// unSafeCopyOfStoredData creates maps copy of storedData. Is not thread safe, you should use `s.lock`.
func (s *LocalStore) unSafeCopyOfStoredData() map[string]*StoredData {
	return maps.Clone(s.storedData)
}
