# TRUSTED-COMPUTING-BASE — Lux Relay

> What you must trust below the relay's signing-profile gate. Companion
> to `PROOF-CLAIMS.md` (the narrow claim) and `LEGACY-CLASSICAL.md` (the
> opt-in classical disclosure).

## §1 Primitive TCBs

| Layer | What you trust | Why |
|---|---|---|
| `luxfi/crypto/mldsa` | FIPS 204 wrapper over Cloudflare CIRCL's ML-DSA-65 reference. KAT-validated. Pinned via `go.mod`. | Verifies the actual ML-DSA-65 signature math. The relay does NOT reimplement any FIPS 204 step. |
| `crypto/ed25519` (Go stdlib) | RFC 8032 Ed25519 reference. | Only used on the classical opt-in path. Under strict-PQ it is unreachable from `vm.verifyReceiptSignature`. |
| `crypto/sha256` (Go stdlib) | FIPS 180-4. | Used for the receipt message canonical-bytes construction and Merkle leaf hashing. |

## §2 Codebase TCB

| Component | Trust | Mitigation |
|---|---|---|
| `pkg/profile/profile.go` | The gate function. Reviewed in `CRYPTOGRAPHER-SIGN-OFF.md`. | ~190 LOC, single function `Permit` is the only policy decision. No conditional re-admission paths. |
| `vm/vm.go::verifyReceiptSignature` | The SINGLE caller of `profile.Verify` for receipts. | Reviewed; no fallback path to ed25519 outside profile. |
| `vm/vm.go::RegisterNodePublicKey` | Stores scheme tag alongside pubkey. | Refuses unknown schemes at registration time; accepts a classical key registration even under strict-PQ but the verifier still refuses it (registration vs verification separation by design). |
| `pkg/zaptransport/zaptransport.go` | Wraps `luxfi/zap`. Carries opaque receipt bytes. Verification is NOT this package's job. | Reviewed; the package never invokes `profile.Verify` or any primitive directly. |

## §3 External-chain TCB (NOT part of the strict-PQ surface)

The relay carries verified messages to external chains. For those:

| Surface | Primitive | Trust source |
|---|---|---|
| Bitcoin / OP_NET | secp256k1 / BIP340 Schnorr (Taproot) | `luxfi/mpc` FROST/Taproot signer; the relay hands off and does not produce signatures. |
| Ethereum | secp256k1 ECDSA | Destination contract's verifying logic; the relay submits transactions but does not vouch for them. |
| Future EVM L2s | per-chain native | Same as Ethereum. |

These are NOT in the strict-PQ TCB. They are out of scope for the
PQ-default guarantee. `LEGACY-CLASSICAL.md` does NOT cover them either
— it only covers the intra-Lux opt-in for operator receipts.

## §4 Build TCB

| Layer | Trust | Reproducibility |
|---|---|---|
| Go 1.26.x toolchain | `go.mod` pin | `GOWORK=off go build ./...` is bit-stable for a given GOTOOLCHAIN. |
| `luxfi/crypto v1.18.5+` | KAT-validated PQ wrappers. | Module proxy pinned via `go.sum`. |
| `luxfi/zap` (current minor) | ZAP transport with optional PQ-TLS-1.3. | Pinned via `go.sum`. |

## §5 What the TCB does NOT include

- **The destination chain's finality**: a re-org on Bitcoin or Ethereum
  invalidates a delivered message in the destination chain's sense, not
  the relay's sense.
- **The luxfi/mpc threshold scheme's soundness**: trusted via FROST /
  CGGMP21 audits in `luxfi/mpc`.
- **The R-Chain consensus protocol**: trusted via the Quasar /
  Pulsar / Corona stack — the relay consumes R-Chain's accepted-block
  guarantee but does not re-prove it.
