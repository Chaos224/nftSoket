package paperkey

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenerateRestoreDeterministic(t *testing.T) {
	k, err := Generate(Words24)
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Fields(k.Mnemonic)) != 24 {
		t.Fatalf("expected 24 words, got %d", len(strings.Fields(k.Mnemonic)))
	}

	// Restoring the same mnemonic + passphrase must yield the same master key.
	r, err := Restore(k.Mnemonic)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if err := k.Unlock("hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := r.Unlock("hunter2"); err != nil {
		t.Fatal(err)
	}
	mk1, _ := k.MasterKey()
	mk2, _ := r.MasterKey()
	if !bytes.Equal(mk1, mk2) {
		t.Fatal("master keys differ for identical mnemonic+passphrase")
	}
}

func TestPassphraseChangesKey(t *testing.T) {
	k, _ := Generate(Words24)
	_ = k.Unlock("alpha")
	a, _ := k.MasterKey()
	_ = k.Unlock("beta")
	b, _ := k.MasterKey()
	if bytes.Equal(a, b) {
		t.Fatal("different passphrases must produce different master keys")
	}
}

func TestSubkeyDomainSeparation(t *testing.T) {
	k, _ := Generate(Words12)
	_ = k.Unlock("")
	enc, _ := k.Subkey("enc", 32)
	addr, _ := k.Subkey("addr", 32)
	if bytes.Equal(enc, addr) {
		t.Fatal("subkeys with different info must differ")
	}
	if len(enc) != 32 {
		t.Fatalf("expected 32-byte subkey, got %d", len(enc))
	}
}

func TestRestoreRejectsBadMnemonic(t *testing.T) {
	if _, err := Restore("not a real mnemonic phrase at all nope"); err == nil {
		t.Fatal("invalid mnemonic should be rejected")
	}
}

func TestMustUnlockFirst(t *testing.T) {
	k, _ := Generate(Words12)
	if _, err := k.MasterKey(); err == nil {
		t.Fatal("MasterKey before Unlock should error")
	}
}

func TestRenderSheetContainsWords(t *testing.T) {
	k, _ := Generate(Words24)
	_ = k.Unlock("")
	sheet, err := k.RenderSheet()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range strings.Fields(k.Mnemonic) {
		if !strings.Contains(sheet, w) {
			t.Fatalf("sheet missing word %q", w)
		}
	}
	if strings.Contains(sheet, "PAPER ACCESS KEY") == false {
		t.Fatal("sheet missing header")
	}
}
