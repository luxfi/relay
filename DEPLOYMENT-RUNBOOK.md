# Deployment Runbook — luxfi/relay

> Operational guidance for running `relayd` against an R-Chain
> (`relayvm`) deployment.

## Audience

- Operators bringing up cross-chain relay between Lux and an external
  chain (Bitcoin, Ethereum, OP_NET).
- Security reviewers validating production posture against the
  cryptographer sign-off (`CRYPTOGRAPHER-SIGN-OFF.md`).

## Quick start

```sh
RELAYD_OPERATOR_ID=NodeID-... \
RELAYD_RELAYVM_RPC=http://127.0.0.1:9650/ext/bc/R/rpc \
RELAYD_LISTEN=:7700 \
RELAYD_ZAP_PORT=7710 \
relayd
```

## Configuration

| Env / flag | Default | Meaning |
|---|---|---|
| `RELAYD_LISTEN` / `--listen` | `:7700` | HTTP listen address (operator API). |
| `RELAYD_ZAP_PORT` / `--zap-port` | `7710` | Intra-Lux ZAP operator-plane port. `0` disables. |
| `RELAYD_RELAYVM_RPC` / `--relayvm-rpc` | `http://127.0.0.1:9650/ext/bc/R/rpc` | R-Chain JSON-RPC URL. |
| `RELAYD_DATA_DIR` / `--data-dir` | `data` | Persistent state directory. |
| `RELAYD_OPERATOR_ID` / `--operator-id` | (required) | This operator's NodeID. |
| `RELAYD_LOG_LEVEL` / `--log-level` | `info` | `debug|info|warn|error`. |

## Signing-profile decision (load-bearing)

The relay defaults to **strict-PQ**: only ML-DSA-65 receipts are accepted
by the R-Chain VM. Validators verify against the operator's registered
ML-DSA-65 public key.

### Strict-PQ (recommended, default)

R-Chain genesis config:
```json
{
  "config": {
    "legacyClassicalEnabled": false
  }
}
```

All operators MUST register ML-DSA-65 keys via the appropriate R-Chain
administrative path. Classical receipts are refused by
`profile.Verify` before any classical signature math runs — see
`pkg/profile/profile.go::Permit`.

### Legacy classical (opt-in only, migration period)

If a deployment has not yet finished migrating operator keys from
Ed25519, set:

```json
{
  "config": {
    "legacyClassicalEnabled": true
  }
}
```

This admits Ed25519 receipts alongside ML-DSA-65. See
`LEGACY-CLASSICAL.md` for the disclosure and the migration plan.

## External-chain interactions

External chains are NOT subject to the PQ-default rule. Specifically:

- **Bitcoin / OP_NET destinations**: relayd hands the verified payload
  off to `luxfi/mpc` for FROST/Taproot threshold signing. The signature
  produced is BIP340 Schnorr, as required by Bitcoin consensus.
- **Ethereum destinations**: relayd submits transactions signed by the
  destination chain's expected key (secp256k1 ECDSA) — typically also
  threshold-signed through `luxfi/mpc`.

These are external-chain primitives. No PQ flip applies to them.

## Operational checks

```sh
# Health
curl -fsS http://127.0.0.1:7700/v1/health | jq

# Status (uptime, channels, pending/relayed counters)
curl -fsS http://127.0.0.1:7700/v1/status | jq

# ZAP peers
curl -fsS http://127.0.0.1:7700/v1/zap/peers | jq

# Pending messages
curl -fsS 'http://127.0.0.1:7700/v1/messages?state=pending' | jq

# Manual re-dispatch
curl -fsS -X POST http://127.0.0.1:7700/v1/relay/trigger \
  -H 'Content-Type: application/json' \
  -d '{"messageId":"<id>"}' | jq
```

## Disaster-recovery

The relay's authoritative state lives on R-Chain. `relayd`'s local
state (`$RELAYD_DATA_DIR/relay.state`) is a courier cache only:

1. Lose the cache → relayd re-polls R-Chain and rebuilds it.
2. Lose the operator's signing key → register a new ML-DSA-65 key on
   R-Chain; rotate via the appropriate administrative path; the old
   key's receipts stop verifying immediately.

Do not edit `relay.state` manually. It is JSON, but its contents are
operator-internal — there is no spec invariant on its shape.
