# PROOF-CLAIMS — Lux Relay

> What this submission DOES and DOES NOT prove. Read this before reading
> any test, runbook, or sign-off.

## §1 The narrow claim

The strongest precise statement supported by the relay codebase at this
revision:

> **PQ-default + classical-gate.** Under the trusted-computing base in
> `TRUSTED-COMPUTING-BASE.md`, every receipt verification call on the
> default `profile.Policy` (zero value) refuses classical schemes
> BEFORE any classical signature math runs, and accepts ML-DSA-65
> signatures iff the underlying `mldsa65.Verify` (FIPS 204 §3) returns
> accept for the receipt's domain-separated input.

This is not a security proof of ML-DSA-65 itself — that proof belongs to
FIPS 204 / Bellare-Rogaway-style EUF-CMA reductions for module-LWE
signatures. We prove that the relay correctly *uses* the FIPS 204
primitive and correctly *refuses* the classical path under strict-PQ.

## §2 Evidence

The narrow claim is supported by, in decreasing order of strength:

1. **Code surface size**: `pkg/profile/profile.go` is ~190 LOC; the gate
   is a single function `Permit(scheme) → error` and is the only
   policy decision in the package.
2. **Single-point-of-entry**: a `grep` over the relay tree shows
   `vm.verifyReceiptSignature` is the ONLY caller of `profile.Verify`,
   and no other code-path invokes `mldsa.PublicKey.Verify*` or
   `ed25519.Verify` directly. (Verify with
   `grep -rn 'ed25519\.Verify\|VerifySignature' relay/`.)
3. **End-to-end tests**:
   - `vm/receipt_pq_test.go::TestE2E_PQReceipt_DefaultMLDSA65` — happy
     path produces a verifying ML-DSA-65 receipt under the default policy.
   - `vm/receipt_pq_test.go::TestStrictPQ_RefusesEd25519Receipt` —
     a structurally valid Ed25519 receipt is refused before any
     classical math runs (the refusal returns `ErrClassicalRefused`).
   - `vm/receipt_pq_test.go::TestLegacyEnabled_AcceptsEd25519Receipt` —
     the opt-in toggle works.
   - `vm/receipt_pq_test.go::TestUnregisteredNode_Refused` — closes the
     prior "log-and-accept" soundness bug.
   - `pkg/profile/profile_test.go` — primitive-level tests.
4. **Fuzzing**: `vm/receipt_fuzz_test.go::FuzzReceiptDecode` and
   `FuzzMessageDecode` exercise the JSON decode boundary; no panics
   after ~50k execs.
5. **ZAP end-to-end**:
   `pkg/zaptransport/zaptransport_test.go::TestZAPTransport_BroadcastReceipt`
   runs two ZAP nodes with `NoDiscovery: true` over loopback and
   round-trips a real JSON receipt.

## §3 What we do NOT prove

- **FIPS 204 EUF-CMA**: ML-DSA-65 itself. Trust comes from FIPS 204 +
  Cloudflare CIRCL's KAT-verified implementation, used through
  `luxfi/crypto/mldsa`.
- **External-chain finality**: the relay accepts a Merkle proof against
  the source-chain root, but the source chain's finality model is
  outside the relay's trust footprint.
- **MPC threshold signing for external destinations**: handed off to
  `luxfi/mpc`. The relay relies on `luxfi/mpc`'s own soundness proofs
  (FROST, CGGMP21).
- **Side-channels on ML-DSA-65**: we use the upstream Cloudflare CIRCL
  implementation; its constant-time evidence is its own. Relay does
  not introduce secret-dependent control flow on top.
- **Receipt-commit Merkle tree forgery resistance**: trusted via
  collision resistance of SHA-256; not separately proven here.

## §4 Future tightening

- Lean / EasyCrypt machine-checked refinement of the
  `Permit → Verify → mldsa65.Verify` chain (currently informal).
- A property-based test asserting `for all scheme ≠ MLDSA65 :
  Verify(Policy{}, scheme, …) returns ErrClassicalRefused` exhaustively
  over the wire-stable enum.
- TLA+ model of the channel state machine for the conflict-freedom
  property (`(destChain, sequence)` uniqueness under concurrent
  vertices).
