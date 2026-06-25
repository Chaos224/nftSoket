// Package paperkey generates and restores the human-portable "paper access key"
// used to unlock a vault, in the style of a cryptocurrency wallet seed phrase.
//
// The access key is a BIP39 mnemonic (12 or 24 words). From the mnemonic (plus
// an optional passphrase, the "25th word") a 64-byte seed is derived, and from
// the seed the symmetric master key. The mnemonic is the ONLY secret the user
// must keep; it can be written on paper and never has to touch the network.
package paperkey

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	bip39 "github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/hkdf"
)

// Strength selects mnemonic length.
type Strength int

const (
	Words12 Strength = 128 // 12 words, 128-bit entropy
	Words24 Strength = 256 // 24 words, 256-bit entropy (recommended)
)

// AccessKey is the in-memory representation of a generated or restored key.
type AccessKey struct {
	Mnemonic string // space-separated words; the only thing to write on paper
	seed     []byte // 64-byte BIP39 seed (kept private)
}

// Generate creates a fresh random access key at the given strength.
func Generate(s Strength) (*AccessKey, error) {
	if s != Words12 && s != Words24 {
		return nil, errors.New("paperkey: strength must be Words12 or Words24")
	}
	entropy, err := bip39.NewEntropy(int(s))
	if err != nil {
		return nil, fmt.Errorf("paperkey: entropy: %w", err)
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return nil, fmt.Errorf("paperkey: mnemonic: %w", err)
	}
	return &AccessKey{Mnemonic: mnemonic}, nil
}

// Restore rebuilds an access key from a written mnemonic, validating the
// checksum so typos are caught before any data is touched.
func Restore(mnemonic string) (*AccessKey, error) {
	mnemonic = normalize(mnemonic)
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, errors.New("paperkey: invalid mnemonic (checksum or word error)")
	}
	return &AccessKey{Mnemonic: mnemonic}, nil
}

// Unlock derives the seed from the mnemonic and optional passphrase. Call this
// before MasterKey/Subkey. The passphrase adds plausible deniability: a wrong
// passphrase yields a different, valid-looking vault rather than an error.
func (k *AccessKey) Unlock(passphrase string) error {
	k.seed = bip39.NewSeed(k.Mnemonic, passphrase)
	return nil
}

// MasterKey returns a 32-byte master key derived from the seed via HKDF.
func (k *AccessKey) MasterKey() ([]byte, error) {
	if len(k.seed) == 0 {
		return nil, errors.New("paperkey: call Unlock before MasterKey")
	}
	return k.Subkey("nftvault/master", 32)
}

// Subkey derives a domain-separated key of the requested length from the seed.
// Different `info` strings yield independent keys (encryption, addressing, MAC).
func (k *AccessKey) Subkey(info string, length int) ([]byte, error) {
	if len(k.seed) == 0 {
		return nil, errors.New("paperkey: call Unlock before deriving keys")
	}
	out := make([]byte, length)
	r := hkdf.New(sha256.New, k.seed, []byte("nftvault-v1-salt"), []byte(info))
	if _, err := r.Read(out); err != nil {
		return nil, fmt.Errorf("paperkey: hkdf: %w", err)
	}
	return out, nil
}

// Fingerprint is a short, non-secret identifier for the vault, safe to display
// so a user can confirm which key/passphrase combination they unlocked.
func (k *AccessKey) Fingerprint() (string, error) {
	pub, err := k.Subkey("nftvault/fingerprint", 8)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%X-%X", pub[:4], pub[4:]), nil
}

func normalize(m string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(m))), " ")
}
