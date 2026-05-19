# Lux Relay — Overview

`luxfi/relay` is the cross-chain message relay for the Lux network. It is
NOT a cryptographic primitive submission — it is operational
infrastructure consuming the primitives that ship in `luxfi/crypto`,
`luxfi/pulsar`, `luxfi/corona`, and the `luxfi/mpc` threshold stack.

This OVERVIEW is the cover sheet for the Tier-A documentation set. It
mirrors the Pulsar / Corona / Magnetar tier-A shape, but the moral
content is different: relay is a courier and a state machine, not an
algorithm.

## Components

| Surface | Layer | Role |
|---|---|---|
| `vm/` | R-Chain VM (relayvm) | Consensus + state for verified cross-chain messages. Source of truth. |
| `pkg/relay/` | Operator engine | Polls source chains, packages events as R-Chain `SendMessage` calls, watches for verified messages, dispatches to destinations. |
| `pkg/server/` | HTTP + ZAP front-ends | `:7700` HTTP for human ops; `:7710` ZAP for intra-Lux operator-to-operator gossip. |
| `pkg/profile/` | Signing-profile gate | ONE function (`profile.Verify`) — strict-PQ default. |
| `pkg/zaptransport/` | Intra-Lux transport | `github.com/luxfi/zap` wrapper for operator-plane traffic. |
| `cmd/relayd/` | Daemon | Standalone operator process. No luxd validator required. |

The chain VM (`vm/`) is the security boundary. `relayd` is just the
courier; it never decides what is verified.

## Two surfaces, one rule

Relay talks to TWO surfaces, and we decomplect them deliberately:

1. **Intra-Lux operator surface** — signed receipts, channel
   attestations, operator-to-operator broadcast.
   **PQ by default**: ML-DSA-65 (FIPS 204). Classical Ed25519 is opt-in
   only via `Config.LegacyClassicalEnabled`. Transport: ZAP.

2. **External-chain surface** — Bitcoin RPC, Ethereum RPC, OP_NET, and
   FROST/Taproot threshold signing handed off to `luxfi/mpc`.
   **Native primitives**: secp256k1, Schnorr-BIP340, Ed25519 — whatever
   the target chain demands. Not subject to the PQ default. The relay
   never PQ-flips an external transaction.

## Tier-A documents

| File | Purpose |
|---|---|
| `OVERVIEW.md` | This file. |
| `SPEC.md` | Protocol, state machine, message format. |
| `PROOF-CLAIMS.md` | Honest scope: what we DO and do NOT prove. |
| `TRUSTED-COMPUTING-BASE.md` | Trust footprint. |
| `CRYPTOGRAPHER-SIGN-OFF.md` | Independent review (this revision). |
| `DEPLOYMENT-RUNBOOK.md` | Operator-facing guidance. |
| `LEGACY-CLASSICAL.md` | Disclosure of the opt-in classical path. |
| `CHANGELOG.md` | Substantive changes to the spec and trust footprint. |

## License

Apache-2.0. See `LICENSE`.
