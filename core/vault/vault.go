// Package vault is the high-level API: it turns a paper access key plus a
// content-addressed store into an encrypted, chunked, redundant file vault.
//
// Flow for storing a file:
//  1. split the file into fixed-size chunks;
//  2. encrypt each chunk under a key derived from the access key;
//  3. write every chunk to all storage backends (>= 2 points);
//  4. build a manifest listing the chunk addresses, encrypt it, and store it;
//  5. return the manifest's address ("root CID") — the only thing the user must
//     remember to retrieve the file (and even that can be re-derived/listed).
package vault

import (
	"encoding/json"
	"fmt"
	"io"

	"nftvault/aead"
	"nftvault/cas"
	"nftvault/paperkey"
)

const (
	chunkSize = 1 << 20 // 1 MiB

	kindChunk    uint32 = 1
	kindManifest uint32 = 2
)

// Manifest describes one stored file.
type Manifest struct {
	Name   string   `json:"name"`
	Size   int64    `json:"size"`
	Chunks []string `json:"chunks"` // content addresses, in order
}

// Vault binds an unlocked access key to a redundant store.
type Vault struct {
	store       *cas.Store
	encKey      []byte
	fingerprint string
}

// Open derives the encryption key and fingerprint from an unlocked access key.
func Open(ak *paperkey.AccessKey, store *cas.Store) (*Vault, error) {
	encKey, err := ak.Subkey("nftvault/chunk-enc", 32)
	if err != nil {
		return nil, fmt.Errorf("vault: derive enc key: %w", err)
	}
	fp, err := ak.Fingerprint()
	if err != nil {
		return nil, fmt.Errorf("vault: fingerprint: %w", err)
	}
	return &Vault{store: store, encKey: encKey, fingerprint: fp}, nil
}

// Fingerprint returns the non-secret vault identifier.
func (v *Vault) Fingerprint() string { return v.fingerprint }

// PutFile streams r, encrypts and stores it, and returns the manifest address.
func (v *Vault) PutFile(name string, r io.Reader) (string, error) {
	m := Manifest{Name: name}
	buf := make([]byte, chunkSize)
	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			env, sealErr := aead.Seal(v.encKey, buf[:n], aead.AAD(v.fingerprint, kindChunk))
			if sealErr != nil {
				return "", fmt.Errorf("vault: seal chunk: %w", sealErr)
			}
			addr, putErr := v.store.Put(env)
			if putErr != nil {
				return "", fmt.Errorf("vault: store chunk: %w", putErr)
			}
			m.Chunks = append(m.Chunks, addr)
			m.Size += int64(n)
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("vault: read: %w", err)
		}
	}

	raw, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	env, err := aead.Seal(v.encKey, raw, aead.AAD(v.fingerprint, kindManifest))
	if err != nil {
		return "", fmt.Errorf("vault: seal manifest: %w", err)
	}
	root, err := v.store.Put(env)
	if err != nil {
		return "", fmt.Errorf("vault: store manifest: %w", err)
	}
	return root, nil
}

// GetManifest fetches and decrypts a manifest by its address.
func (v *Vault) GetManifest(root string) (*Manifest, error) {
	env, err := v.store.Get(root)
	if err != nil {
		return nil, fmt.Errorf("vault: get manifest: %w", err)
	}
	raw, err := aead.Open(v.encKey, env, aead.AAD(v.fingerprint, kindManifest))
	if err != nil {
		return nil, fmt.Errorf("vault: decrypt manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("vault: parse manifest: %w", err)
	}
	return &m, nil
}

// GetFile retrieves, decrypts, and writes a stored file to w.
func (v *Vault) GetFile(root string, w io.Writer) (*Manifest, error) {
	m, err := v.GetManifest(root)
	if err != nil {
		return nil, err
	}
	for i, addr := range m.Chunks {
		env, err := v.store.Get(addr)
		if err != nil {
			return nil, fmt.Errorf("vault: get chunk %d: %w", i, err)
		}
		pt, err := aead.Open(v.encKey, env, aead.AAD(v.fingerprint, kindChunk))
		if err != nil {
			return nil, fmt.Errorf("vault: decrypt chunk %d: %w", i, err)
		}
		if _, err := w.Write(pt); err != nil {
			return nil, fmt.Errorf("vault: write chunk %d: %w", i, err)
		}
	}
	return m, nil
}
