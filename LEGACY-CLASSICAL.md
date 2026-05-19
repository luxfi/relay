# LEGACY-CLASSICAL — luxfi/relay

> Disclosure of the opt-in classical (Ed25519) path on the relay's
> intra-Lux operator surface. Required by the Tier-A discipline.

## §1 Scope of this disclosure

This document covers ONE thing: the `Config.LegacyClassicalEnabled`
toggle on R-Chain and `profile.Policy.LegacyClassicalEnabled` on
relayd, which together control whether **Ed25519** receipts are
accepted on the intra-Lux operator surface.

This document does NOT cover:

- External-chain primitives (Bitcoin secp256k1 Schnorr, Ethereum
  secp256k1 ECDSA, etc.). Those are dictated by the target chain and
  are not "legacy" in this sense — they are the native primitive of
  their respective chains.
- ML-DSA-65 itself, which is the default. See `SPEC.md` §5 and
  `PROOF-CLAIMS.md` §1 for the strict-PQ guarantee.

## §2 Why the toggle exists

During the migration of Lux operator infrastructure from
Ed25519-based operator identities to ML-DSA-65 operator identities,
some validator sets are mid-flight. The toggle exists so a chain can
opt into accepting Ed25519 receipts from operators that have not yet
finished rotating their keys.

The toggle is OFF by default. New deployments default to strict-PQ.

## §3 What changes when the toggle is on

| Behaviour | `LegacyClassicalEnabled=false` (default) | `LegacyClassicalEnabled=true` |
|---|---|---|
| `profile.SchemeMLDSA65` receipts | accepted iff `mldsa65.Verify` accepts | accepted iff `mldsa65.Verify` accepts |
| `profile.SchemeEd25519` receipts | refused with `ErrClassicalRefused` BEFORE any classical math | accepted iff `ed25519.Verify` accepts with `ContextTag` prepended |
| Operator key registration | accepts both schemes (registration is not the gate) | accepts both schemes |
| Domain-separation context | `"luxfi.relay.v1"` (FIPS 204 §5.2 ctx for ML-DSA-65; prepended bytes for Ed25519) | same |

## §4 Domain separation under classical

When Ed25519 is permitted, the bytes actually fed to `ed25519.Sign` /
`ed25519.Verify` are:

```
tagged = "luxfi.relay.v1" || message
```

This guarantees a classical relay receipt cannot be replayed as some
other Lux artifact (e.g. an oracle observation, which uses
`"luxfi.oracle.v1"`). The protection is structural domain
separation, NOT cryptographic novelty.

## §5 Migration plan (suggested)

1. **Phase A (bootstrap)**: deploy R-Chain with
   `legacyClassicalEnabled: true`. Operators sign receipts with
   whichever scheme their key material supports.
2. **Phase B (migration)**: operators generate ML-DSA-65 keys and
   register them alongside their Ed25519 keys (registration accepts
   both schemes irrespective of policy). Operators begin signing new
   receipts with ML-DSA-65.
3. **Phase C (cutover)**: all operators confirmed migrated.
   Re-deploy R-Chain with `legacyClassicalEnabled: false`. Existing
   Ed25519 receipts stop verifying immediately.
4. **Phase D (cleanup)**: deregister Ed25519 keys.

Phases C → D require a quorum-coordinated configuration change. There
is no fast cut-over path that does not break in-flight Ed25519
receipts; that is deliberate — surprise-PQ-flip would be a soundness
risk, not a feature.

## §6 What the toggle does NOT enable

- **Cross-scheme signature substitution**: an Ed25519 key registered
  under `SchemeEd25519` cannot produce a receipt that verifies under
  `SchemeMLDSA65`, and vice versa, because `profile.Verify` dispatches
  on the scheme tag and the verifier-side check uses the registered
  pubkey's scheme.
- **Hybrid signatures**: there is no concatenation-of-classical-and-PQ
  receipt format in this codebase. Each receipt carries exactly one
  signature under exactly one scheme.
- **External-chain PQ flip**: out of scope (see §1).
