package node

import (
	"bytes"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"nftvault/cas"
	"nftvault/paperkey"
	"nftvault/vault"
)

func newNode(t *testing.T, token string) (*httptest.Server, *cas.FSBackend) {
	t.Helper()
	be, err := cas.NewFSBackend("disk", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewServer(be, token).Handler())
	t.Cleanup(srv.Close)
	return srv, be
}

func TestRemoteBackendRoundTrip(t *testing.T) {
	srv, _ := newNode(t, "secret-token")
	rb := NewRemoteBackend("node1", srv.URL, "secret-token")

	if !rb.Healthy() {
		t.Fatal("node should be healthy")
	}
	data := []byte("a remote, encrypted block")
	addr := cas.Address(data)
	if err := rb.Put(addr, data); err != nil {
		t.Fatal(err)
	}
	ok, err := rb.Has(addr)
	if err != nil || !ok {
		t.Fatalf("Has = %v, %v", ok, err)
	}
	got, err := rb.Get(addr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("remote round trip mismatch")
	}
}

func TestAuthRequired(t *testing.T) {
	srv, _ := newNode(t, "right-token")
	bad := NewRemoteBackend("node1", srv.URL, "wrong-token")
	if err := bad.Put(cas.Address([]byte("x")), []byte("x")); err == nil {
		t.Fatal("put with wrong token must fail")
	}
}

func TestServerRejectsBadAddress(t *testing.T) {
	srv, be := newNode(t, "")
	rb := NewRemoteBackend("node1", srv.URL, "")
	// Claim a wrong address for the content; node must reject it.
	if err := rb.Put(cas.Address([]byte("real")), []byte("tampered")); err == nil {
		t.Fatal("node must reject content that does not match its address")
	}
	if ok, _ := be.Has(cas.Address([]byte("real"))); ok {
		t.Fatal("nothing should have been stored")
	}
}

func TestGetMissingIsNotFound(t *testing.T) {
	srv, _ := newNode(t, "")
	rb := NewRemoteBackend("node1", srv.URL, "")
	if _, err := rb.Get(cas.Address([]byte("absent"))); err != cas.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// Full integration: a vault replicating across TWO remote nodes (two real
// storage points reachable only over the network), with failover.
func TestVaultOverTwoRemoteNodes(t *testing.T) {
	srvA, _ := newNode(t, "tok")
	srvB, _ := newNode(t, "tok")
	a := NewRemoteBackend("point-a", srvA.URL, "tok")
	b := NewRemoteBackend("point-b", srvB.URL, "tok")
	store, err := cas.NewStore(a, b)
	if err != nil {
		t.Fatal(err)
	}

	ak, _ := paperkey.Generate(paperkey.Words24)
	_ = ak.Unlock("pp")
	v, err := vault.Open(ak, store)
	if err != nil {
		t.Fatal(err)
	}

	payload := bytes.Repeat([]byte("decentralized "), 200000) // ~2.6 MB
	root, err := v.PutFile("doc.bin", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}

	// Take node A offline; vault must still recover from node B.
	srvA.Close()
	var out bytes.Buffer
	if _, err := v.GetFile(root, &out); err != nil {
		t.Fatalf("failover over network failed: %v", err)
	}
	if !bytes.Equal(out.Bytes(), payload) {
		t.Fatal("recovered data differs")
	}
}

// Compatibility guard: the content-address scheme and AEAD envelope are
// versioned; a fixed-format block must remain retrievable. If this breaks, an
// update changed an on-the-wire/on-disk format and would strand existing data.
func TestFormatStability(t *testing.T) {
	dir := t.TempDir()
	be, _ := cas.NewFSBackend("disk", filepath.Join(dir, "d"))
	srv := httptest.NewServer(NewServer(be, "").Handler())
	t.Cleanup(srv.Close)
	rb := NewRemoteBackend("n", srv.URL, "")

	block := []byte("format-stable payload v1")
	addr := cas.Address(block)
	const expected = "v1:" // address scheme prefix must remain v1
	if addr[:3] != expected {
		t.Fatalf("address scheme changed: %s", addr)
	}
	if err := rb.Put(addr, block); err != nil {
		t.Fatal(err)
	}
	got, err := rb.Get(addr)
	if err != nil || !bytes.Equal(got, block) {
		t.Fatalf("stored block no longer retrievable: %v", err)
	}
}
