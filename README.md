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

## Self-hosted storage nodes (Phase 2)

Storage points can be **your own networked nodes** instead of local folders, so
the system is genuinely decentralized while staying 100% self-owned — nothing
depends on a third-party service. A node only ever holds encrypted,
content-addressed blocks (zero-trust): even a compromised node cannot read your
data, and cannot tamper with it undetected (address == hash is checked on both
write and read).

```bash
# Run two storage points (on localhost, your servers, anywhere you control)
export NFTVAULT_NODE_TOKEN="a-long-shared-secret"
./vaultnode --listen 127.0.0.1:8443 --data ./nodeA --name nodeA &
./vaultnode --listen 127.0.0.1:8444 --data ./nodeB --name nodeB &
# (in production pass --tls-cert/--tls-key)

# Point the vault at them
export NFTVAULT_NODES="http://127.0.0.1:8443,http://127.0.0.1:8444"
./vaultctl put ./somefile          # replicated across both nodes
./vaultctl get <root> --out ./restored
```

If one node goes down, reads transparently fail over to the survivor and
re-replicate when it returns. Verified live (kill a node mid-flight, data still
recovers).

## Billing & space management (Phase 3)

A self-hosted **control server** (`vaultserver`) manages tenants, the storage
quota each plan offers, live usage, and billing — with no external database
(state is a local JSON file) and no hard dependency on a payment processor
(payment is a pluggable interface; the default is a manual/offline ledger).

```bash
# Run the control plane
export NFTVAULT_ADMIN_TOKEN="a-long-admin-secret"
./vaultserver --listen 127.0.0.1:9000 --state ./control.json &

# Operator: define a plan and create an account (returns a one-time token)
export NFTVAULT_CONTROL=http://127.0.0.1:9000
./vaultadmin plan-create --name Demo --quota 4M --price 999 --currency EUR
./vaultadmin account-create --name Alice --plan <plan_id>   # -> token vlt_...

# Storage nodes enforce that account's quota by consulting the control server
./vaultnode --listen :8643 --data ./nodeA --control http://127.0.0.1:9000 &
./vaultnode --listen :8644 --data ./nodeB --control http://127.0.0.1:9000 &

# Client uses the account token; writes over quota are refused (HTTP 507)
export NFTVAULT_NODE_TOKEN=vlt_...   NFTVAULT_ACCOUNT_TOKEN=vlt_...
./vaultctl account     # plan, space used/free, billing status

# Operator billing operations
./vaultadmin billing-run                 # issue invoices for ended periods
./vaultadmin billing-enforce --grace-hours 168   # suspend unpaid past grace
./vaultadmin invoice-pay --id <invoice_id>       # settle, reactivate account
```

What's enforced: an account that is suspended or over its plan's `quota_bytes`
cannot store more (verified live and in tests). Quota counts the bytes physically
stored across nodes (e.g. 2 replicas of 1 MiB use ~2 MiB of offered space).
Enforcement happens on the provider's infrastructure (the node), not on trust in
the client. *Known limitation:* a file that crosses the quota mid-upload stores
its chunks-so-far before the refusing chunk; whole-file pre-reservation/rollback
is a planned refinement.

### Payment middleware (pluggable: card now, crypto later)

Payment is a **middleware**, not a monolith: one `payment.Provider` interface +
a registry, with each method in its own subpackage (`payment/stripe`,
`payment/bitcoin`, `payment/manual`). The domain and control plane depend only on
the interface; concrete providers are wired in at the composition root
(`cmd/vaultserver`) from environment variables. Adding a method = one new
subpackage + one `Register(...)` — nothing else changes, so methods can be added
and improved zone by zone.

```bash
# Enable card payments (Stripe) and on-chain Bitcoin by setting env on the server
export STRIPE_SECRET_KEY=sk_live_...   STRIPE_WEBHOOK_SECRET=whsec_...
export BITCOIN_ADDRESS=bc1q...         BITCOIN_WEBHOOK_SECRET=...  BITCOIN_MIN_CONF=2
./vaultserver --listen :9000 --state ./control.json     # payments=[bitcoin manual stripe]

# Operator can issue an ad-hoc invoice (e.g. a 4.99 EUR / 10 GB top-up)
./vaultadmin invoice-create --id <account_id> --amount 499

# Customer pays by card (Stripe Checkout) or crypto (BIP21)
./vaultctl invoices
./vaultctl checkout --invoice <id> --provider stripe    # -> hosted checkout URL
./vaultctl checkout --invoice <id> --provider bitcoin   # -> address + bitcoin: URI
```

Confirmation is by **signed webhook**: Stripe's `checkout.session.completed`
(verified via the `Stripe-Signature` HMAC, with replay-window protection) or a
chain watcher's HMAC-signed callback once Bitcoin reaches `MIN_CONF`
confirmations. Forged/misrouted callbacks are rejected (HTTP 400) and never
settle an invoice. The full card flow — checkout → signed webhook → invoice paid,
and forged webhook rejected — is verified live and in tests. *(Stripe = card
payments today; Bitcoin provider is structurally complete and pluggable, pending
a production address-derivation source and chain watcher. Prices are examples;
e.g. 4.99 EUR / 10 GB — finalize later.)*

## Stability — updates must not break stored data

All persisted/transmitted formats are explicitly versioned and frozen behind a
contract (see [`core/FORMAT.md`](core/FORMAT.md)), enforced by a build-failing
compatibility test. Dependencies are **vendored** (`core/vendor/`) so the project
builds offline and an upstream change cannot silently break it.

## Roadmap

- **Phase 1 — Security core** ✅
  paper key, key schedule, AEAD, content-addressed redundant store, Shamir
  sharing, `vaultctl` CLI, full test suite.
- **Phase 2 — Self-hosted decentralized storage** ✅
  `vaultnode` daemon + `RemoteBackend` client, token auth, optional TLS,
  zero-trust nodes, network failover & self-heal, vendored deps, format contract.
- **Phase 3 — Control server: billing & space management** ✅
  self-hosted `vaultserver` (accounts, plans, quota, usage, invoices, payments;
  local JSON state), `vaultadmin` operator CLI, node-side quota enforcement
  (opt-in `--control`), `vaultctl account`.
- **Phase 3b — Payment middleware** ✅
  pluggable `payment.Provider` + registry; Stripe (card) and Bitcoin (crypto)
  and manual providers in independent subpackages; checkout + signed-webhook
  settlement; `vaultctl invoices/checkout`, `vaultadmin invoice-create/providers`.
- **Phase 2b / 3c — next** *(planned)*
  erasure coding (Reed–Solomon m-of-n) instead of full replication; node
  discovery; Argon2id passphrase stretching; key rotation; whole-file quota
  pre-reservation; mutual-TLS between nodes and control; audit log; production
  Bitcoin address-derivation + chain watcher.
- **Phase 4 — Native clients**
  Windows desktop app (encrypted virtual drive), plus macOS/Linux/Android/iOS/web
  sharing the same `core` over a stable API.

See [`core/README.md`](core/README.md) for package-level details.
```
