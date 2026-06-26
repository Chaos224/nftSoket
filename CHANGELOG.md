# Changelog

## v0.1.0-beta — first public beta

First beta of NFT Vault: a self-hosted, encrypted, decentralized storage system
unlocked by a paper access key. Everything below is implemented and tested
(`cd core && go test ./...`); builds offline from vendored dependencies.

### Security core
- BIP39 paper access key (12/24 words) + optional passphrase; printable sheet.
- HKDF key schedule with domain separation; vault fingerprint.
- XChaCha20-Poly1305 authenticated encryption (versioned envelope, AAD binding).
- Content-addressed store (BLAKE2b-256 of ciphertext): private by construction.
- Shamir secret sharing (k-of-n) to split the master key across custodians.

### Decentralized storage
- `vaultnode`: self-hosted storage points you run anywhere.
- Replication to ≥2 points, read failover, integrity-verified reads, self-heal.
- Zero-trust nodes: they only ever hold encrypted, content-addressed blocks.

### Control plane — billing & space management
- Accounts, plans, storage-quota ("offered space") enforcement, usage.
- Subscription billing: invoices, payments, delinquency suspend/reactivate.
- Local JSON state (no external database); `vaultserver` + `vaultadmin`.

### Payment middleware (pluggable)
- One `payment.Provider` interface + registry; providers in independent
  subpackages, wired at the composition root.
- Stripe (card) via Checkout + signed-webhook settlement.
- Bitcoin (crypto): BIP21 + HMAC-signed chain-watcher webhook (structure ready;
  needs a production address source + watcher to go live).
- Manual (offline) default.

### Stability & supply chain
- All formats versioned behind a compatibility contract (`core/FORMAT.md`).
- Dependencies vendored; offline reproducible build.
- Removed the earlier non-compiling `nftclient/` + `server/` prototype, which
  was the source of the repository's 2 moderate Dependabot alerts. The shipping
  `core/` module uses current, non-flagged dependency versions.

### Not yet (planned)
- Native GUI clients (Windows desktop with encrypted virtual drive; mobile/web).
- Erasure coding (Reed–Solomon) instead of full replication.
- Production Bitcoin address derivation + chain watcher; key rotation;
  whole-file quota pre-reservation; mutual-TLS between nodes and control.

> Note: this beta is tagged in git history as `v0.1.0-beta` locally; the version
> is also recorded in the `VERSION` file because the hosting environment's git
> policy blocks pushing tag refs from this session.
