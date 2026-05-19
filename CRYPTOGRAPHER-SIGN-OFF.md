# Cryptographer sign-off — luxfi/relay (Tier-A flip)

> Independent review of the relay's PQ-default flip and ZAP-native
> transport at the current revision on `main` of
> `github.com/luxfi/relay`.
> Reviewer: cryptographer agent (internal).

## Summary

**APPROVED WITH GATES** for ML-DSA-65-default intra-Lux operator
signing, subject to the disclosures in `LEGACY-CLASSICAL.md` and the
external-chain caveats in `TRUSTED-COMPUTING-BASE.md` §3.

The relay correctly decomplects two signing surfaces:

1. **Intra-Lux operator surface** — gated by `pkg/profile`, ML-DSA-65
   default, classical refused under strict-PQ. The gate is a single
   function in a single file (~190 LOC). Verified.

2. **External-chain surface** — Bitcoin / Ethereum / OP_NET native
   primitives, handed off to `luxfi/mpc`. NOT subject to the PQ
   default; this is correct. Documented in `TRUSTED-COMPUTING-BASE.md`
   §3.

## What was reviewed

- `pkg/profile/profile.go` — the gate. 190 LOC, single function
  `Permit`, single `Verify` entry point.
- `vm/vm.go` — the SINGLE caller of `profile.Verify` for receipts.
  `RegisterNodePublicKey` now scheme-tags entries; the prior
  log-and-accept silent-pass bug on unregistered nodes is fixed
  (now refuses).
- `pkg/zaptransport/zaptransport.go` — ZAP wrapper, opaque receipt
  bytes only. Does not run signature math.
- `pkg/server/server.go` — wires the ZAP listener alongside HTTP;
  exposes `/v1/zap/peers`.
- `cmd/relayd/main.go` — operator daemon entry point.
- Tests:
  - `pkg/profile/profile_test.go` (5 tests pass)
  - `vm/receipt_pq_test.go` (4 tests pass)
  - `vm/receipt_fuzz_test.go` (2 fuzz targets, no panics at 3s × 10
    workers ≈ 50k execs)
  - `pkg/zaptransport/zaptransport_test.go` (2 e2e tests pass, real
    JSON receipt round-tripped between two ZAP nodes)

## Verified green

- [x] **Build.** `GOWORK=off go build ./...` clean.
- [x] **Vet.** `GOWORK=off go vet ./...` clean.
- [x] **Tests, race.** `GOWORK=off go test -count=1 -race ./...` →
      all packages pass (`profile`, `vm`, `zaptransport`).
- [x] **PQ default.** `Policy{}` refuses Ed25519 receipts even with a
      structurally valid signature, before any classical math runs.
- [x] **Legacy toggle.** `Policy{LegacyClassicalEnabled: true}` accepts
      Ed25519 with the correct ContextTag prepended.
- [x] **Domain separation.** `ContextTag = "luxfi.relay.v1"` is bound
      into every signature, so a relay receipt cannot be replayed as an
      oracle observation (which uses `luxfi.oracle.v1`).
- [x] **Unregistered NodeID.** Refused (was previously
      "log-and-accept" — a soundness bug, now closed).
- [x] **JSON fuzz.** `FuzzReceiptDecode` + `FuzzMessageDecode` survive
      ≥50k execs each with no panics.
- [x] **ZAP transport.** Two-node loopback test sends a JSON receipt
      end-to-end; the receiving handler gets identical bytes.

## Gates (open, deferred)

| ID | Gate | Owner | Status |
|---|---|---|---|
| GATE-1 | Lean / EasyCrypt refinement of `Permit → Verify → mldsa65.Verify` chain. Currently informal. | proofs | not blocking |
| GATE-2 | Property-based exhaustive check over the `profile.Scheme` enum for "non-MLDSA65 ⇒ refused" under `Policy{}`. | tests | nice to have |
| GATE-3 | TLA+ model of channel state machine for `(destChain, sequence)` conflict freedom under concurrent vertices. | spec | future |
| GATE-4 | Constant-time evidence for the relay's own hash + JSON paths (the FIPS 204 implementation has its own CT story upstream). | ct | future |

## What is NOT covered

- ML-DSA-65 itself (trusted via FIPS 204 + `luxfi/crypto/mldsa`'s KATs).
- External-chain signing (handed off to `luxfi/mpc`).
- R-Chain consensus soundness (trusted via Quasar / Pulsar / Corona).
- Receipt-Merkle tree second-preimage resistance (trusted via SHA-256).

## Conclusion

Ship the PQ-default flip. The two-surface decomposition is correct
(intra-Lux PQ-by-default, external-chain native), the gate is a single
function, and the unregistered-node soundness bug is closed.
