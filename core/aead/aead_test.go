package aead

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func key(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, keySize)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	k := key(t)
	aad := AAD("ABCD-1234", 1)
	msg := []byte("decentralized, encrypted, content-addressed")
	env, err := Seal(k, msg, aad)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Open(k, env, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatal("round trip mismatch")
	}
}

func TestWrongKeyFails(t *testing.T) {
	env, _ := Seal(key(t), []byte("x"), nil)
	if _, err := Open(key(t), env, nil); err == nil {
		t.Fatal("wrong key must fail to open")
	}
}

func TestWrongAADFails(t *testing.T) {
	k := key(t)
	env, _ := Seal(k, []byte("x"), AAD("vault-a", 1))
	if _, err := Open(k, env, AAD("vault-b", 1)); err == nil {
		t.Fatal("mismatched AAD must fail")
	}
}

func TestTamperDetected(t *testing.T) {
	k := key(t)
	env, _ := Seal(k, []byte("important"), nil)
	env[len(env)-1] ^= 0xFF // flip a tag bit
	if _, err := Open(k, env, nil); err == nil {
		t.Fatal("tampered ciphertext must fail authentication")
	}
}

func TestNonceIsRandom(t *testing.T) {
	k := key(t)
	a, _ := Seal(k, []byte("same"), nil)
	b, _ := Seal(k, []byte("same"), nil)
	if bytes.Equal(a, b) {
		t.Fatal("two seals of identical plaintext must differ (random nonce)")
	}
}
