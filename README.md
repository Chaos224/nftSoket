# NFT Vault — secure, decentralized, encrypted storage

A system for storing data **encrypted, content-addressed, and redundantly across
at least two independent storage points**, unlocked by a **paper access key** in
the style of a cryptocurrency wallet. The long-term goal is a fast, decentralized
("improved IPFS") backend with a hardened server and native clients for every
platform.

This repository is being built in phases. **Phase 1 — the security core — is
implemented, tested, and working today** (see [`core/`](core/)). The earlier
`nftclient/` and `server/` directories are an initial prototype that does not
compile; they are kept for reference and are superseded by `core/` (and by the
hardened server planned in Phase 3).

---

## Why this design

Your requirements map directly onto concrete, proven cryptographic mechanisms:

| Requirement (ro) | Mechanism | Where |
|---|---|---|
| „cheie de acces pe foaie, ca la crypto valet" | BIP39 mnemonic (12/24 words) + optional passphrase → seed → keys | [`core/paperkey`](core/paperkey) |
| „minim 2 puncte unde se păstrează informația" | (a) every block replicated to ≥2 backends; (b) the master key itself split with Shamir's Secret Sharing (k-of-n) | [`core/cas`](core/cas), [`core/shamir`](core/shamir) |
| „folosim ideea IPFS dar o îmbunătățim la maxim" | content-addressing **of ciphertext** (encryption-first), integrity-verified reads, self-healing replication, multi-backend | [`core/cas`](core/cas) |
| „totul securizat" | XChaCha20-Poly1305 AEAD, per-purpose key derivation (HKDF), context binding (AAD), nothing secret written to disk | [`core/aead`](core/aead), [`core/vault`](core/vault) |

### How it improves on plain IPFS

- **Private by construction.** Vanilla IPFS addresses are hashes of *plaintext*,
  so the network can see and dedup your content. Here the address is the hash of
  the *ciphertext*; the storage layer never sees plaintext and cannot correlate
  your data with anyone else's.
- **Guaranteed redundancy.** A write is only acknowledged once ≥2 storage points
  hold the block — not "maybe pinned somewhere."
- **Integrity on every read.** The address is re-verified against the returned
  bytes, so a malicious or faulty backend cannot substitute data.
- **Self-healing.** Reads re-replicate any block that a backend has lost.

---

## Architecture

```
                 paper access key (BIP39 mnemonic + optional passphrase)
                                   │
                          ┌────────┴────────┐
                          │   key schedule  │  HKDF domain separation
                          │  (paperkey/kdf) │
                          └───┬─────────┬───┘
            master key ◄──────┘         └──────► chunk-enc key, fingerprint
                 │                                        │
        ┌────────┴─────────┐                     ┌────────┴─────────┐
        │  Shamir split     │                     │  vault            │
        │  k-of-n shares    │                     │  chunk → encrypt  │
        │  (custodians)     │                     │  → manifest       │
        └───────────────────┘                     └────────┬─────────┘
                                                           │ ciphertext blocks
                                              ┌────────────┴────────────┐
                                              │  content-addressed store │
                                              │  (cas) replicate + heal  │
                                              └──────┬───────────┬───────┘
                                                 point A      point B   …  (≥2)
```

---

## Try it now

```bash
cd core
go test ./...                      # all packages pass
go build -o vaultctl ./cmd/vaultctl

# 1. Generate a paper access key (printable sheet, shown once)
./vaultctl keygen --words 24

# 2. Store a file encrypted across two points (prompts for mnemonic + passphrase,
#    or set NFTVAULT_MNEMONIC / NFTVAULT_PASSPHRASE)
./vaultctl put ./somefile --vault ./vault-data
#   → prints a root address ("CID") and the vault fingerprint

# 3. Retrieve and decrypt it
./vaultctl get <root> --out ./restored --vault ./vault-data

# 4. See which storage points hold the data
./vaultctl health <root> --vault ./vault-data

# 5. Split the master key into 2-of-3 custodian shares (and recombine)
./vaultctl split --shares 3 --threshold 2
./vaultctl combine   # paste any 2 shares
```

A full end-to-end run (store 2.6 MB → ciphertext-only on two points → delete one
point → recover from the survivor → self-heal) is exercised by the test suite and
was verified manually.

---

## Security model (Phase 1)

- The mnemonic (+ optional passphrase) is the root secret; everything else is
  derived. It is never persisted by the tools.
- Keys are domain-separated via HKDF, so the encryption key, addressing, and
  fingerprint are cryptographically independent.
- Each block is sealed with XChaCha20-Poly1305 (24-byte random nonce) and bound
  via AAD to the vault fingerprint, so blocks cannot be transplanted between
  vaults.
- The passphrase acts as a BIP39 "25th word": a wrong passphrase yields a
  different, valid-looking vault rather than an error (plausible deniability).
- Secrets are read from stdin (no-echo) or env vars — never from CLI flags.

What Phase 1 does **not** yet cover (see roadmap): networked storage points,
authentication/transport to a server, key rotation, erasure coding, and a GUI.

---

## Roadmap

- **Phase 1 — Security core** ✅ *(this commit)*
  paper key, key schedule, AEAD, content-addressed redundant store, Shamir
  sharing, `vaultctl` CLI, full test suite.
- **Phase 2 — Decentralized storage network**
  remote storage backends over authenticated TLS/Noise; erasure coding
  (e.g. Reed–Solomon m-of-n) instead of plain replication; gossip/DHT discovery;
  pinning incentives.
- **Phase 3 — Hardened server**
  replaces the legacy `server/`: mutual-TLS, Argon2id-hashed credentials,
  capability tokens, rate limiting, audit log, no plaintext secrets.
- **Phase 4 — Native clients**
  Windows desktop app (encrypted virtual drive), plus macOS/Linux/Android/iOS/web
  sharing the same `core` over a stable API.

See [`core/README.md`](core/README.md) for package-level details.
```
