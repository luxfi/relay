// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package zaptransport wraps github.com/luxfi/zap as the intra-Lux operator
// transport for relayd.
//
// Decomplecting: relayd talks to TWO surfaces.
//
//  1. External chains (Bitcoin RPC, Ethereum RPC, ...) — HTTP/JSON-RPC, native
//     primitive. This is NOT a ZAP surface.
//
//  2. Other relay operators within Lux — ZAP. ZAP carries sealed,
//     PQ-TLS-1.3-friendly frames between operator nodes for sharing
//     in-flight cross-chain Messages and SignedReceipts before R-Chain has
//     committed them.
//
// This package is purely a transport — verification is the VM's job and
// stays gated by pkg/profile.
package zaptransport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/luxfi/zap"
)

// Message types reserved on the relay ZAP plane. Wire-stable enum.
//
// ZAP encodes the message type in the upper byte of the wire flags field
// (`msgType << 8`), so the type space is 1 byte. We carve out 0x48..0x4F
// for relay-plane traffic. Values are uint16 to match zap.Node.Handle.
const (
	// MsgRelayHello — opening handshake.
	MsgRelayHello uint16 = 0x48

	// MsgRelayMessageBroadcast — operator-to-operator pre-commit broadcast of
	// a SignedReceipt envelope. The payload is the JSON-encoded receipt.
	// The receiving operator verifies it through the VM's verifyReceiptSignature
	// gate before doing anything else with it.
	MsgRelayMessageBroadcast uint16 = 0x49

	// MsgRelayStatusPing — liveness probe between operator peers.
	MsgRelayStatusPing uint16 = 0x4A
)

// ServiceType is the mDNS service tag for relay operators.
const ServiceType = "_luxd-relay._tcp"

// Config configures a relay-plane ZAP node.
type Config struct {
	NodeID      string
	Port        int
	NoDiscovery bool
	Logger      *slog.Logger
}

// Node is a thin facade over zap.Node so the relay package never imports
// the ZAP wire layer directly.
type Node struct {
	z   *zap.Node
	log *slog.Logger
}

// New constructs a relay-plane ZAP node. Start must be called before use.
func New(cfg Config) (*Node, error) {
	if cfg.NodeID == "" {
		return nil, errors.New("zaptransport: NodeID required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	z := zap.NewNode(zap.NodeConfig{
		NodeID:      cfg.NodeID,
		ServiceType: ServiceType,
		Port:        cfg.Port,
		NoDiscovery: cfg.NoDiscovery,
		Logger:      cfg.Logger,
	})
	return &Node{z: z, log: cfg.Logger}, nil
}

// Start brings the node online.
func (n *Node) Start() error { return n.z.Start() }

// Stop shuts the node down.
func (n *Node) Stop() { n.z.Stop() }

// NodeID returns this operator's ZAP node identifier.
func (n *Node) NodeID() string { return n.z.NodeID() }

// Peers lists currently connected relay operator peers.
func (n *Node) Peers() []string { return n.z.Peers() }

// HandleReceipt installs h as the consumer of inbound MsgRelayMessageBroadcast
// frames. The handler receives the JSON-encoded receipt bytes; it is the
// handler's job to decode and run them through profile.Verify.
func (n *Node) HandleReceipt(h func(ctx context.Context, from string, receipt []byte) error) {
	n.z.Handle(MsgRelayMessageBroadcast, func(ctx context.Context, from string, msg *zap.Message) (*zap.Message, error) {
		body, err := receiptBody(msg)
		if err != nil {
			return nil, err
		}
		if err := h(ctx, from, body); err != nil {
			return nil, err
		}
		return nil, nil
	})
}

// BroadcastReceipt sends receipt bytes to every connected peer.
// Each peer independently verifies via profile.Verify; this transport
// does not run any signature math itself.
func (n *Node) BroadcastReceipt(ctx context.Context, receipt []byte) map[string]error {
	msg, err := buildReceiptMessage(receipt)
	if err != nil {
		return map[string]error{"_build": err}
	}
	return n.z.Broadcast(ctx, msg)
}

// ConnectDirect adds an unannounced peer (e.g. via static config).
func (n *Node) ConnectDirect(addr string) error { return n.z.ConnectDirect(addr) }

// receiptBody extracts the payload from a ZAP message. We use the
// builder's GetBytes accessor through the public zap API to avoid pulling
// internal struct types.
func receiptBody(msg *zap.Message) ([]byte, error) {
	root := msg.Root()
	if root.IsNull() {
		return nil, errors.New("zaptransport: empty zap message")
	}
	// Root field 0 carries the receipt JSON bytes.
	b := root.Bytes(0)
	if len(b) == 0 {
		return nil, errors.New("zaptransport: zero-length receipt payload")
	}
	// Receipt body is itself JSON; quickly sanity-check that it parses.
	var probe map[string]any
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, fmt.Errorf("zaptransport: receipt body not JSON: %w", err)
	}
	return b, nil
}

// buildReceiptMessage frames receipt bytes as a ZAP MsgRelayMessageBroadcast.
// ZAP encodes the message type in the upper byte of the wire flags field
// (`msgType << 8`), which is how the node's handler dispatch reads it back.
func buildReceiptMessage(receipt []byte) (*zap.Message, error) {
	b := zap.NewBuilder(len(receipt) + 64)
	ob := b.StartObject(8)
	ob.SetBytes(0, receipt)
	ob.FinishAsRoot()
	flags := uint16(MsgRelayMessageBroadcast) << 8
	data := b.FinishWithFlags(flags)
	return zap.Parse(data)
}
