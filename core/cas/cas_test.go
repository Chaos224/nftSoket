package cas

import (
	"bytes"
	"path/filepath"
	"sync"
	"testing"
)

// memBackend is an in-memory Backend for tests; failable to simulate outages.
type memBackend struct {
	name string
	mu   sync.Mutex
	data map[string][]byte
	fail bool
}

func newMem(name string) *memBackend {
	return &memBackend{name: name, data: map[string][]byte{}}
}
func (m *memBackend) Name() string { return m.name }
func (m *memBackend) Put(addr string, data []byte) error {
	if m.fail {
		return ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	m.data[addr] = cp
	return nil
}
func (m *memBackend) Get(addr string) ([]byte, error) {
	if m.fail {
		return nil, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.data[addr]
	if !ok {
		return nil, ErrNotFound
	}
	return d, nil
}
func (m *memBackend) Has(addr string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[addr]
	return ok, nil
}

func TestAddressDeterministic(t *testing.T) {
	if Address([]byte("hello")) != Address([]byte("hello")) {
		t.Fatal("address not deterministic")
	}
	if Address([]byte("a")) == Address([]byte("b")) {
		t.Fatal("distinct data shares an address")
	}
}

func TestStoreNeedsTwoBackends(t *testing.T) {
	if _, err := NewStore(newMem("only")); err == nil {
		t.Fatal("single backend should be rejected")
	}
}

func TestReplicationAndFailover(t *testing.T) {
	a, b := newMem("a"), newMem("b")
	s, err := NewStore(a, b)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := s.Put([]byte("redundant block"))
	if err != nil {
		t.Fatal(err)
	}
	// Both backends should hold it.
	if ha, _ := a.Has(addr); !ha {
		t.Fatal("backend a missing block")
	}
	if hb, _ := b.Has(addr); !hb {
		t.Fatal("backend b missing block")
	}
	// Knock out a: read must still succeed from b.
	a.fail = true
	got, err := s.Get(addr)
	if err != nil {
		t.Fatalf("failover get: %v", err)
	}
	if !bytes.Equal(got, []byte("redundant block")) {
		t.Fatal("failover returned wrong data")
	}
}

func TestSelfHeal(t *testing.T) {
	a, b := newMem("a"), newMem("b")
	s, _ := NewStore(a, b)
	addr, _ := s.Put([]byte("heal me"))
	// Simulate backend b losing the block.
	delete(b.data, addr)
	if _, err := s.Get(addr); err != nil {
		t.Fatal(err)
	}
	if ok, _ := b.Has(addr); !ok {
		t.Fatal("Get did not re-replicate to healed backend")
	}
}

func TestFSBackendRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a, err := NewFSBackend("disk-a", filepath.Join(dir, "a"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewFSBackend("disk-b", filepath.Join(dir, "b"))
	if err != nil {
		t.Fatal(err)
	}
	s, _ := NewStore(a, b)
	addr, err := s.Put([]byte("on disk in two places"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(addr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("on disk in two places")) {
		t.Fatal("fs round trip mismatch")
	}
}
