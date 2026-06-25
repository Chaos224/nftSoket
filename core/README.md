# core — NFT Vault security core (`module nftvault`)

Phase 1 of the system: a tested, dependency-light Go module implementing the
cryptographic and storage foundation. No network, no GUI yet — just the parts
everything else must be built on, done correctly.

## Packages

| Package | Responsibility |
|---|---|
| `paperkey` | BIP39 mnemonic generation/restore, seed + key derivation (HKDF), printable paper sheet, vault fingerprint. The "crypto-wallet paper key." |
| `shamir` | Shamir's Secret Sharing over GF(256): split a secret into n shares, any k reconstruct. Used to spread the master key across custodians. |
| `aead` | XChaCha20-Poly1305 authenticated encryption with a versioned envelope and associated-data binding. |
| `cas` | Content-addressed store. `Address` = BLAKE2b-256 of the (cipher)bytes. `Store` replicates to ≥2 `Backend`s, fails over, verifies integrity, self-heals. `FSBackend` is a sharded on-disk backend. |
| `vault` | High-level API: chunk a file, encrypt each chunk, store redundantly, write an encrypted manifest, return a root address; and the reverse. |
| `node` | Self-hosted storage point: an HTTP daemon serving `cas.Backend` (versioned `/v1` protocol, bearer-token auth, server-side integrity) plus `RemoteBackend`, a `cas.Backend` client. Zero-trust: nodes see only ciphertext. |
| `cmd/vaultctl` | Reference CLI tying it all together (local dirs or `--node` remotes). |
| `cmd/vaultnode` | The storage-node daemon (`--listen`, `--data`, `--tls-cert/key`, `NFTVAULT_NODE_TOKEN`). |

## Develop

```bash
go test ./...          # unit tests for shamir, paperkey, aead, cas, vault
go vet ./...
go build ./...
go build -o vaultctl ./cmd/vaultctl
```

## Dependencies

- `github.com/tyler-smith/go-bip39` — BIP39 mnemonics
- `golang.org/x/crypto` — argon2 (reserved for passphrase stretching), chacha20poly1305, blake2b, hkdf
- `golang.org/x/term` — no-echo passphrase entry

## Stability

Formats are versioned and frozen behind a contract — see [`FORMAT.md`](FORMAT.md).
Dependencies are vendored, so `GOFLAGS=-mod=vendor GOPROXY=off go build ./...`
works with no network.

## Notes / next steps within core

- `cas.Store` currently does full replication; a future erasure-coded backend
  (Reed–Solomon) will let n points tolerate the loss of any n−k without storing
  full copies everywhere.
- `aead` envelope is versioned (byte 0) so the format can evolve without breaking
  stored data.
- The legacy top-level `nftclient/` and `server/` directories predate this module
  and do not compile; they will be replaced by Phases 3–4.
```
