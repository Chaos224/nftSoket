package shamir

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestSplitCombineRoundTrip(t *testing.T) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}

	shares, err := Split(secret, 5, 3)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(shares) != 5 {
		t.Fatalf("expected 5 shares, got %d", len(shares))
	}

	// Any 3 of the 5 must recover the secret.
	combos := [][]int{{0, 1, 2}, {0, 2, 4}, {1, 3, 4}, {2, 3, 4}}
	for _, c := range combos {
		subset := []Share{shares[c[0]], shares[c[1]], shares[c[2]]}
		got, err := Combine(subset)
		if err != nil {
			t.Fatalf("combine %v: %v", c, err)
		}
		if !bytes.Equal(got, secret) {
			t.Fatalf("combine %v: secret mismatch", c)
		}
	}
}

func TestThresholdNotReachedProducesWrongSecret(t *testing.T) {
	secret := []byte("a 2-of-3 secret value here ok!!!")
	shares, err := Split(secret, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	// Only 2 shares for a threshold-3 secret: must NOT equal the secret.
	got, err := Combine(shares[:2])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(got, secret) {
		t.Fatal("recovered secret with fewer than threshold shares")
	}
}

func TestTwoOfTwo(t *testing.T) {
	secret := []byte("two storage points minimum")
	shares, err := Split(secret, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Combine(shares)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatal("2-of-2 round trip failed")
	}
}

func TestInvalidParams(t *testing.T) {
	if _, err := Split([]byte("x"), 2, 1); err == nil {
		t.Fatal("threshold 1 should fail")
	}
	if _, err := Split([]byte("x"), 1, 2); err == nil {
		t.Fatal("parts < threshold should fail")
	}
	if _, err := Split(nil, 3, 2); err == nil {
		t.Fatal("empty secret should fail")
	}
}

func TestDuplicateShareRejected(t *testing.T) {
	secret := []byte("dedupe x check")
	shares, _ := Split(secret, 3, 2)
	dup := []Share{shares[0], shares[0]}
	if _, err := Combine(dup); err == nil {
		t.Fatal("duplicate x coordinate should be rejected")
	}
}
