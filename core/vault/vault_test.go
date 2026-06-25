package vault

import (
	"bytes"
	"crypto/rand"
	"path/filepath"
	"testing"

	"nftvault/cas"
	"nftvault/paperkey"
)

func newTestVault(t *testing.T, passphrase string) *Vault {
	t.Helper()
	ak, err := paperkey.Generate(paperkey.Words24)
	if err != nil {
		t.Fatal(err)
	}
	if err := ak.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	a, _ := cas.NewFSBackend("a", filepath.Join(dir, "a"))
	b, _ := cas.NewFSBackend("b", filepath.Join(dir, "b"))
	store, err := cas.NewStore(a, b)
	if err != nil {
		t.Fatal(err)
	}
	v, err := Open(ak, store)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestPutGetRoundTrip(t *testing.T) {
	v := newTestVault(t, "pp")
	// Multi-chunk payload (~2.5 MiB) to exercise chunk boundaries.
	data := make([]byte, 2_500_000)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	root, err := v.PutFile("big.bin", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	m, err := v.GetFile(root, &out)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "big.bin" {
		t.Fatalf("name = %q", m.Name)
	}
	if m.Size != int64(len(data)) {
		t.Fatalf("size = %d want %d", m.Size, len(data))
	}
	if len(m.Chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(m.Chunks))
	}
	if !bytes.Equal(out.Bytes(), data) {
		t.Fatal("retrieved data differs from stored data")
	}
}

func TestWrongKeyCannotDecrypt(t *testing.T) {
	v := newTestVault(t, "right")
	root, err := v.PutFile("secret.txt", bytes.NewReader([]byte("classified")))
	if err != nil {
		t.Fatal(err)
	}

	// A different vault (different key) pointed at the same store must fail.
	other := newTestVault(t, "wrong")
	other.store = v.store // same backends, different encKey/fingerprint
	if _, err := other.GetManifest(root); err == nil {
		t.Fatal("foreign key must not decrypt the manifest")
	}
}

func TestEmptyFile(t *testing.T) {
	v := newTestVault(t, "")
	root, err := v.PutFile("empty", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	m, err := v.GetFile(root, &out)
	if err != nil {
		t.Fatal(err)
	}
	if m.Size != 0 || out.Len() != 0 {
		t.Fatal("empty file round trip failed")
	}
}
