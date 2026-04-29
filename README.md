# Lux Relay

Cross-chain message relay for the Lux network. Two surfaces:

- **`vm/`** — `relayvm` chain VM (R-Chain). Imports as a luxd plugin or standalone chain. Runs the consensus + state layer for verified cross-chain channels and messages. Source of truth.
- **`cmd/relayd/`** — standalone operator daemon. Polls source chains for events, packages them as R-Chain `SendMessage` calls, watches for verified messages, and dispatches to destinations. Same operational shape as `mpcd` and `kms` — no luxd validator required.

The chain VM is the security boundary; `relayd` is just the courier.

## relayd

```sh
RELAYD_OPERATOR_ID=NodeID-... \
RELAYD_RELAYVM_RPC=http://127.0.0.1:9650/ext/bc/R/rpc \
relayd
```

HTTP endpoints (default `:7700`):

| Path | Method | Purpose |
|---|---|---|
| `/v1/health` | GET | health probe |
| `/v1/status` | GET | uptime, channel count, message counters |
| `/v1/channels` | GET | list R-Chain channels tracked by this operator |
| `/v1/messages?state=pending` | GET | list in-flight messages |
| `/v1/relay/trigger` | POST | re-attempt delivery of a specific message ID |

State persists at `$RELAYD_DATA_DIR/relay.state` (JSON).

## relayvm (chain VM)

`vm/` is the canonical relay VM. Import path `github.com/luxfi/relay/vm`.
The luxd plugin wrapper lives in `~/work/lux/chains/relayvm/` and just
re-exports this package.

## Architecture

```
┌─ source chain ─┐         ┌─ R-Chain (relayvm) ─┐         ┌─ dest chain ─┐
│ Outbound event │ ─poll─► │ SendMessage         │ ─poll─► │ Inbound call │
└────────────────┘         │ GetVerifiedMessage  │         └──────────────┘
                           └─ relayd operator ───┘
```

Recipients carried as `bytes32` so the same primitive covers EVM
(address as uint160 in low bytes) and Bitcoin / OP_NET (32-byte Taproot
x-only pubkey) uniformly. For non-EVM destinations, `relayd` hands the
verified payload to `luxfi/mpc` for FROST/Taproot threshold signing.
