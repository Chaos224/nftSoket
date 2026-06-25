// Package aead provides authenticated encryption for vault chunks and
// manifests. It uses XChaCha20-Poly1305 (a 24-byte random nonce, so nonce reuse
// is not a practical concern) and a versioned envelope so the format can evolve.
package aead

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	version  = 0x01
	keySize  = chacha20poly1305.KeySize  // 32
	nonceLen = chacha20poly1305.NonceSizeX // 24
)

// Envelope layout: [version:1][nonce:24][ciphertext+tag].
// The associated data (aad) is authenticated but not stored; the caller must
// supply the same aad to Open. We bind the vault fingerprint as aad so chunks
// from one vault cannot be transplanted into another.

// Seal encrypts plaintext with key, authenticating aad.
func Seal(key, plaintext, aad []byte) ([]byte, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("aead: key must be %d bytes", keySize)
	}
	c, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("aead: nonce: %w", err)
	}
	out := make([]byte, 1+nonceLen, 1+nonceLen+len(plaintext)+c.Overhead())
	out[0] = version
	copy(out[1:], nonce)
	out = c.Seal(out, nonce, plaintext, aad)
	return out, nil
}

// Open decrypts an envelope produced by Seal, verifying aad.
func Open(key, envelope, aad []byte) ([]byte, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("aead: key must be %d bytes", keySize)
	}
	if len(envelope) < 1+nonceLen {
		return nil, errors.New("aead: envelope too short")
	}
	if envelope[0] != version {
		return nil, fmt.Errorf("aead: unsupported envelope version %d", envelope[0])
	}
	c, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce := envelope[1 : 1+nonceLen]
	ct := envelope[1+nonceLen:]
	pt, err := c.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, errors.New("aead: authentication failed (wrong key or corrupted data)")
	}
	return pt, nil
}

// AAD builds associated data from a vault fingerprint and a logical purpose,
// giving each ciphertext a context it is cryptographically bound to.
func AAD(fingerprint string, kind uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, kind)
	return append([]byte(fingerprint+"|"), b...)
}
