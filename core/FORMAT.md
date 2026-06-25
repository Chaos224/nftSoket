# On-disk & on-the-wire formats (stability contract)

A core goal of this project is that **updates must never break access to data
that is already stored.** To make that enforceable rather than aspirational,
every persisted or transmitted format is explicitly versioned, and the rules
below are treated as a contract. The `node` package's `TestFormatStability`
fails the build if any of these change incompatibly.

## Versioned formats

| Format | Version marker | Defined in |
|---|---|---|
| Content address | string prefix `v1:` followed by lowercase hex of BLAKE2b-256(bytes) | `cas.Address` |
| Encrypted envelope | first byte `0x01`, then 24-byte nonce, then XChaCha20-Poly1305 ciphertext+tag | `aead` |
| Manifest | JSON object `{name, size, chunks[]}`, itself sealed in an envelope | `vault.Manifest` |
| Node wire protocol | `X-Vault-Api: 1` header; routes under `/v1/...` | `node` |
| Paper access key | BIP39 (12/24 words) + optional passphrase; HKDF salt `nftvault-v1-salt` | `paperkey` |

## Rules for changes

1. **Never repurpose an existing version marker.** To change a format, add a new
   version (`v2:`, envelope byte `0x02`, `/v2/...`) and keep readers for the old
   one. Writers may switch to the new version; readers must accept both.
2. **Addresses are immutable.** A block's address is a hash of its exact bytes;
   blocks are content-addressed and never rewritten in place.
3. **HKDF salt and `info` strings are frozen.** Changing them changes every
   derived key and would orphan existing vaults. New keys get new `info` labels.
4. **The manifest is forward-compatible JSON.** New optional fields may be added;
   existing fields keep their meaning. Unknown fields are ignored on read.
5. **Dependencies are vendored** (`core/vendor/`), so an upstream release cannot
   silently change behavior or break a build. Updating a dep is a deliberate,
   reviewed change.

## Why this matters here

The root secret is a paper key the user may not touch for years. The derivation
(`paperkey`), the encryption (`aead`), and the addressing (`cas`) must produce
byte-for-byte the same results in a future version, or the paper key would stop
working. These three are the most stability-critical and must only ever change
behind a new version marker.
