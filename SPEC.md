# SPEC — Lux Relay (R-Chain + relayd)

> Standalone protocol specification for the Lux cross-chain relay. Covers
> the R-Chain VM state machine, the relayd operator daemon's lifecycle,
> and the wire format of intra-Lux receipts. External-chain RPC formats
> are NOT in scope (they are dictated by the target chain).

## §1 Scope

This document specifies:

1. The R-Chain (`relayvm`) state machine: channels, messages, receipts,
   commitments.
2. The relayd operator daemon's polling, packaging, and dispatch
   lifecycle.
3. The signing-profile gate (`pkg/profile`) and the strict-PQ default.
4. The intra-Lux ZAP transport (`pkg/zaptransport`) used for
   pre-commit operator gossip.

Not in scope:

- The single-party FIPS 204 ML-DSA algorithm (see `luxfi/crypto/mldsa`
  and FIPS 204).
- The Pulsar threshold ML-DSA construction (see `luxfi/pulsar`).
- External-chain transaction encoding (each chain's spec applies).

## §2 Terminology

- **Channel**: a directed pair `(sourceChain, destChain)` opened on
  R-Chain. Channels carry an `ordering` and a monotonically increasing
  per-channel sequence number.
- **Message**: a single cross-chain envelope `(channelID, seq, payload,
  sender, receiver, timeout)`. Messages flow through states `pending →
  verified → delivered`, with `failed` as a sink.
- **VerifiedMessage**: the artifact (`luxfi/node/vms/artifacts`) produced
  when R-Chain accepts a Merkle proof against the source-chain finality
  root.
- **SignedReceipt**: an operator's acknowledgment of message receipt.
  Carries a `Scheme` tag (`profile.Scheme`) and a signature over
  `H(MessageID || SessionID || NodeID || Timestamp || ContentHash)`.
- **ReceiptCommit**: a Merkle root over a session's `SignedReceipt`s,
  committed at block boundaries.

## §3 State machine

```
                          [open]
   OpenChannel ─────────► Channel
                            │
                            │ SendMessage(channelID, payload, ...)
                            ▼
                  Message{state=pending}
                            │
                            │ ReceiveMessage(msgID, proof, sourceHeight)
                            │   ├── proof verifies → state=verified
                            │   └── proof fails    → state=failed
                            ▼
                  Message{state=verified}
                            │
                            │ relayd.deliver / RelayVertex.Accept
                            ▼
                  Message{state=delivered}
```

Conflict key on the DAG layer: `(destChain, sequence)`. Two vertices
conflict iff they share any such pair (vm/dag_vertex.go).

## §4 Wire format

### §4.1 SignedReceipt

```go
SignedReceipt {
    MessageID    ids.ID
    SessionID    [32]byte
    NodeID       ids.NodeID
    Scheme       profile.Scheme   // 0x01 = ml-dsa-65, 0x02 = ed25519
    Timestamp    uint64
    ContentHash  [32]byte
    Signature    []byte           // ml-dsa-65: 3309 bytes; ed25519: 64 bytes
}
```

Signed message:
```
m = SHA-256( MessageID || SessionID || NodeID || BE64(Timestamp) || ContentHash )
ctx = "luxfi.relay.v1"               // FIPS 204 §5.2 ctx for ml-dsa-65
                                     // prepended to m for ed25519
```

### §4.2 ZAP plane

ZAP message types reserved by the relay plane:

| Type | Value | Body |
|---|---|---|
| `MsgRelayHello` | `0x48` | handshake (TBD) |
| `MsgRelayMessageBroadcast` | `0x49` | JSON `SignedReceipt` bytes at field 0 |
| `MsgRelayStatusPing` | `0x4A` | liveness probe |

`msgType << 8` is stored in the ZAP flags field, per the upstream
`luxfi/zap` framing convention. Service tag: `_luxd-relay._tcp`.

## §5 Signing-profile gate

The relay has TWO signing surfaces:

1. **Intra-Lux operator surface** (signed receipts, channel attestations).
   - Default scheme: `profile.SchemeMLDSA65` (FIPS 204 ML-DSA-65).
   - Domain-separation context: `profile.ContextTag = "luxfi.relay.v1"`.
   - Classical Ed25519 opt-in only via `Config.LegacyClassicalEnabled`.

2. **External-chain surface** (Bitcoin / Ethereum / OP_NET RPC, FROST
   handoff to `luxfi/mpc`).
   - Whatever the target chain demands. Not subject to the PQ default.

The single function `profile.Verify(policy, scheme, pub, msg, sig)` is
the gate. `vm.verifyReceiptSignature` calls it once; no other VM code
ever invokes `ed25519.Verify` or any classical primitive directly.

## §6 Lifecycle (relayd)

```
boot → operatorID required (--operator-id or RELAYD_OPERATOR_ID)
     → http listener :7700
     → optional zap listener :7710 (RELAYD_ZAP_PORT)
     → relay engine: 15s ticker, polls R-Chain RPC
poll → fetchChannels()         (RPC: relay.listChannels)
     → fetchVerifiedMessages() (RPC: relay.getVerifiedMessages)
     → deliver(msg)            (per-destination adapter)
     → persistState()          ($RELAYD_DATA_DIR/relay.state)
shut → engine.Shutdown(), zap.Stop(), httpSrv.Shutdown()
```

HTTP endpoints:

| Path | Method | Purpose |
|---|---|---|
| `/v1/health` | GET | health probe |
| `/v1/status` | GET | uptime, channel count, message counters |
| `/v1/channels` | GET | channels tracked by this operator |
| `/v1/messages?state=…` | GET | in-flight messages |
| `/v1/relay/trigger` | POST | re-attempt a specific message ID |
| `/v1/zap/peers` | GET | ZAP plane peers (or `{enabled:false}`) |

## §7 Security goals

- **EUF-CMA receipts**: an adversary without an operator's secret key
  cannot forge a `SignedReceipt` that verifies under that operator's
  registered public key (ML-DSA-65 under FIPS 204 §3, NIST Level 3).
- **No-classical-under-strict-PQ**: under the default policy
  (`Policy{}`), classical signatures are refused by
  `profile.Verify` BEFORE any signature math runs. This is the
  Rich-Hickey "ONE place" gate; no other code-path can re-admit.
- **Domain separation**: `ContextTag = "luxfi.relay.v1"` is bound into
  every relay signature. A relay receipt cannot be replayed as some
  other Lux artifact (e.g. an oracle observation, which uses
  `luxfi.oracle.v1`).
- **Replay resistance**: receipts carry `(MessageID, SessionID,
  Timestamp)`; the verifier checks against the registered NodeID's key.
- **Conflict freedom**: the DAG conflict key `(destChain, sequence)`
  guarantees at most one verified delivery per channel-sequence pair.

Out of scope: external-chain finality assumptions (handled by the
relevant source/destination chain's specs).
