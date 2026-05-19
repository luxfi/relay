# CHANGELOG — luxfi/relay

## Unreleased — Tier-A flip

### Added
- `pkg/profile`: signing-profile gate. ML-DSA-65 (FIPS 204) default;
  Ed25519 opt-in only via `Policy.LegacyClassicalEnabled`. Single
  `Verify` entry point; the gate is one function in one file.
- `pkg/zaptransport`: intra-Lux operator transport over
  `github.com/luxfi/zap`. Carries opaque JSON receipt bytes; never
  invokes signature math itself.
- Tier-A documentation set: `OVERVIEW.md`, `SPEC.md`,
  `PROOF-CLAIMS.md`, `TRUSTED-COMPUTING-BASE.md`,
  `CRYPTOGRAPHER-SIGN-OFF.md`, `DEPLOYMENT-RUNBOOK.md`,
  `LEGACY-CLASSICAL.md`.
- `vm.Config.LegacyClassicalEnabled` — chain-level toggle for the
  classical opt-in.
- `relayd` flags: `--zap-port`, env `RELAYD_ZAP_PORT`. `/v1/zap/peers`
  HTTP endpoint.

### Changed
- `vm.SignedReceipt` carries `Scheme` (`profile.Scheme`). Wire-stable
  enum: `0x01 = ml-dsa-65`, `0x02 = ed25519`.
- `VM.RegisterNodePublicKey(nodeID, scheme, pub)` replaces the
  Ed25519-only signature. Registration accepts both schemes; the
  verifier is the gate.
- `VM.verifyReceiptSignature` is now a thin shim over
  `profile.Verify(policy, scheme, pub, msg, sig)` — no
  primitive-specific code remains in the VM.
- Receipt signatures bind the domain-separation context
  `"luxfi.relay.v1"` (FIPS 204 §5.2 ctx for ML-DSA-65; prepended
  bytes for Ed25519).

### Fixed
- **Soundness fix**: `verifyReceiptSignature` no longer silently
  accepts receipts from unregistered nodes (was "log and accept"; now
  refuses). This was a latent forgery risk.
- ZAP message-type encoding aligned with upstream `luxfi/zap`
  `msgType << 8` flags convention; reserved relay plane in
  `0x48..0x4F`.

### Tests
- `pkg/profile/profile_test.go` (5 tests).
- `vm/receipt_pq_test.go` (4 end-to-end PQ default / classical-refuse /
  classical-opt-in / unregistered tests).
- `vm/receipt_fuzz_test.go` (2 fuzz targets — receipt and message
  decoders, ~50k execs no panics at 3s each).
- `pkg/zaptransport/zaptransport_test.go` (2 end-to-end ZAP
  broadcast tests).
