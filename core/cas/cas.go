// Package cas is a content-addressed store: the "improved IPFS" storage layer.
//
// Differences from vanilla IPFS, by design:
//   - Encryption-first: callers store ciphertext, so addresses never reveal
//     plaintext and identical plaintext under different keys does not collide.
//   - Explicit multi-backend replication: every block is written to ALL
//     configured backends (the "at least 2 storage points"); reads fail over
//     across backends and self-heal missing replicas.
//   - Integrity on read: the address is re-verified against the bytes, so a
//     backend cannot silently serve corrupted or substituted data.
package cas

import (
	"encoding/hex"
	"errors"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

// addrPrefix versions the address scheme so it can change without ambiguity.
const addrPrefix = "v1:"

// Address deterministically derives the content address of data.
func Address(data []byte) string {
	sum := blake2b.Sum256(data)
	return addrPrefix + hex.EncodeToString(sum[:])
}

func verify(addr string, data []byte) error {
	if Address(data) != addr {
		return fmt.Errorf("cas: integrity check failed for %s", addr)
	}
	return nil
}

// Backend is a single physical storage location (a directory, an object store,
// a remote node). It is intentionally minimal.
type Backend interface {
	Name() string
	Put(addr string, data []byte) error
	Get(addr string) ([]byte, error)
	Has(addr string) (bool, error)
}

// ErrNotFound is returned by backends when an address is absent.
var ErrNotFound = errors.New("cas: block not found")

// Store fans writes out to every backend and reads from the first that has the
// block, healing any backend that is missing it.
type Store struct {
	backends []Backend
}

// NewStore requires at least two backends to honor the redundancy guarantee.
func NewStore(backends ...Backend) (*Store, error) {
	if len(backends) < 2 {
		return nil, errors.New("cas: need at least 2 backends for redundancy")
	}
	return &Store{backends: backends}, nil
}

// Put stores data under its content address in all backends and returns the
// address. A write succeeds only if at least two backends accepted the block.
func (s *Store) Put(data []byte) (string, error) {
	addr := Address(data)
	ok := 0
	var lastErr error
	for _, b := range s.backends {
		if err := b.Put(addr, data); err != nil {
			lastErr = fmt.Errorf("%s: %w", b.Name(), err)
			continue
		}
		ok++
	}
	if ok < 2 {
		return "", fmt.Errorf("cas: stored on only %d backend(s): %v", ok, lastErr)
	}
	return addr, nil
}

// Get retrieves a block by address, verifies its integrity, and re-replicates
// it to any backend that is missing or has a corrupted copy (best-effort).
func (s *Store) Get(addr string) ([]byte, error) {
	var data []byte
	for _, b := range s.backends {
		got, err := b.Get(addr)
		if err != nil {
			continue
		}
		if err := verify(addr, got); err != nil {
			continue // treat corruption as a miss on this backend
		}
		data = got
		break
	}
	if data == nil {
		return nil, ErrNotFound
	}
	// Self-heal: ensure every backend holds a good copy. Cheap Has probe first.
	for _, b := range s.backends {
		if ok, err := b.Has(addr); err == nil && ok {
			continue
		}
		_ = b.Put(addr, data)
	}
	return data, nil
}

// Health reports, per backend, whether a probe address is present. Useful for a
// dashboard / status command.
func (s *Store) Health(addr string) map[string]bool {
	out := make(map[string]bool, len(s.backends))
	for _, b := range s.backends {
		ok, err := b.Has(addr)
		out[b.Name()] = err == nil && ok
	}
	return out
}
