package cas

import (
	"errors"
	"os"
	"path/filepath"
)

// FSBackend stores blocks as files in a directory, sharded by address prefix to
// avoid huge flat directories. It is the default local "storage point".
type FSBackend struct {
	name string
	root string
}

// NewFSBackend creates (if needed) the backing directory.
func NewFSBackend(name, root string) (*FSBackend, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &FSBackend{name: name, root: root}, nil
}

func (f *FSBackend) Name() string { return f.name }

func (f *FSBackend) path(addr string) string {
	// addr looks like "v1:<64 hex>"; shard by first 2 hex chars.
	h := addr
	if i := len(addrPrefix); len(addr) > i+2 {
		h = addr[i:]
	}
	return filepath.Join(f.root, h[:2], h)
}

func (f *FSBackend) Put(addr string, data []byte) error {
	p := f.path(addr)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	// Content-addressed blocks are immutable; skip rewrite if present.
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (f *FSBackend) Get(addr string) ([]byte, error) {
	data, err := os.ReadFile(f.path(addr))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return data, err
}

func (f *FSBackend) Has(addr string) (bool, error) {
	_, err := os.Stat(f.path(addr))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
