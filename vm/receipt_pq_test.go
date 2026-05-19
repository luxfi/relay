// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vm

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/luxfi/ids"
	"github.com/luxfi/relay/pkg/profile"
)

// receiptMessage reproduces the receipt-signing message format from
// VM.verifyReceiptSignature so tests can sign over the same bytes.
func receiptMessage(r *SignedReceipt) []byte {
	h := sha256.New()
	h.Write(r.MessageID[:])
	h.Write(r.SessionID[:])
	h.Write(r.NodeID[:])
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], r.Timestamp)
	h.Write(ts[:])
	h.Write(r.ContentHash[:])
	return h.Sum(nil)
}

// TestE2E_PQReceipt_DefaultMLDSA65 exercises the default (strict-PQ) path:
// an operator with an ML-DSA-65 key registers, signs a receipt, and the VM
// accepts it. This is the canonical happy path for intra-Lux signing.
func TestE2E_PQReceipt_DefaultMLDSA65(t *testing.T) {
	vm := &VM{
		nodePublicKeys: make(map[ids.NodeID]nodeKey),
		policy:         profile.Default(),
	}

	signer, err := profile.NewMLDSA65Signer(rand.Reader)
	if err != nil {
		t.Fatalf("new ml-dsa-65 signer: %v", err)
	}

	var nodeID ids.NodeID
	if _, err := rand.Read(nodeID[:]); err != nil {
		t.Fatalf("rand nodeID: %v", err)
	}
	if err := vm.RegisterNodePublicKey(nodeID, profile.SchemeMLDSA65, signer.PublicKey()); err != nil {
		t.Fatalf("register pq key: %v", err)
	}

	var msgID ids.ID
	if _, err := rand.Read(msgID[:]); err != nil {
		t.Fatalf("rand msgID: %v", err)
	}

	r := &SignedReceipt{
		MessageID:   msgID,
		NodeID:      nodeID,
		Scheme:      profile.SchemeMLDSA65,
		Timestamp:   1700000000,
		ContentHash: sha256.Sum256([]byte("payload")),
	}
	sig, err := signer.Sign(receiptMessage(r))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	r.Signature = sig

	if err := vm.verifyReceiptSignature(r); err != nil {
		t.Fatalf("strict-PQ receipt verify: %v", err)
	}
}

// TestStrictPQ_RefusesEd25519Receipt asserts the policy gate fires BEFORE
// any classical signature math: an Ed25519 receipt under default policy
// must be refused even with a perfectly valid signature.
func TestStrictPQ_RefusesEd25519Receipt(t *testing.T) {
	vm := &VM{
		nodePublicKeys: make(map[ids.NodeID]nodeKey),
		policy:         profile.Default(),
	}

	pub, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen ed25519: %v", err)
	}

	var nodeID ids.NodeID
	if _, err := rand.Read(nodeID[:]); err != nil {
		t.Fatalf("rand nodeID: %v", err)
	}
	if err := vm.RegisterNodePublicKey(nodeID, profile.SchemeEd25519, pub); err != nil {
		t.Fatalf("register classical key (registration itself should not refuse): %v", err)
	}

	var msgID ids.ID
	if _, err := rand.Read(msgID[:]); err != nil {
		t.Fatalf("rand msgID: %v", err)
	}
	r := &SignedReceipt{
		MessageID:   msgID,
		NodeID:      nodeID,
		Scheme:      profile.SchemeEd25519,
		Timestamp:   1700000000,
		ContentHash: sha256.Sum256([]byte("payload")),
	}
	// Sign with the matching tag so the signature itself is structurally valid.
	tagged := append([]byte(profile.ContextTag), receiptMessage(r)...)
	r.Signature = ed25519.Sign(sk, tagged)

	if err := vm.verifyReceiptSignature(r); err == nil {
		t.Fatalf("strict-PQ MUST refuse classical receipt")
	}
}

// TestLegacyEnabled_AcceptsEd25519Receipt confirms operators can opt back
// into classical via the LegacyClassicalEnabled config flag.
func TestLegacyEnabled_AcceptsEd25519Receipt(t *testing.T) {
	vm := &VM{
		nodePublicKeys: make(map[ids.NodeID]nodeKey),
		policy:         profile.Policy{LegacyClassicalEnabled: true},
	}

	pub, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen ed25519: %v", err)
	}

	var nodeID ids.NodeID
	if _, err := rand.Read(nodeID[:]); err != nil {
		t.Fatalf("rand nodeID: %v", err)
	}
	if err := vm.RegisterNodePublicKey(nodeID, profile.SchemeEd25519, pub); err != nil {
		t.Fatalf("register classical key: %v", err)
	}

	var msgID ids.ID
	if _, err := rand.Read(msgID[:]); err != nil {
		t.Fatalf("rand msgID: %v", err)
	}
	r := &SignedReceipt{
		MessageID:   msgID,
		NodeID:      nodeID,
		Scheme:      profile.SchemeEd25519,
		Timestamp:   1700000000,
		ContentHash: sha256.Sum256([]byte("payload")),
	}
	tagged := append([]byte(profile.ContextTag), receiptMessage(r)...)
	r.Signature = ed25519.Sign(sk, tagged)

	if err := vm.verifyReceiptSignature(r); err != nil {
		t.Fatalf("legacy-enabled classical receipt verify: %v", err)
	}
}

// TestUnregisteredNode_Refused asserts the prior "log-and-accept" bug
// stays fixed: an unregistered NodeID must NOT silently produce a valid
// receipt.
func TestUnregisteredNode_Refused(t *testing.T) {
	vm := &VM{
		nodePublicKeys: make(map[ids.NodeID]nodeKey),
		policy:         profile.Default(),
	}
	var nodeID ids.NodeID
	if _, err := rand.Read(nodeID[:]); err != nil {
		t.Fatalf("rand nodeID: %v", err)
	}
	r := &SignedReceipt{
		MessageID: ids.GenerateTestID(),
		NodeID:    nodeID,
		Scheme:    profile.SchemeMLDSA65,
		Signature: []byte("anything"),
	}
	if err := vm.verifyReceiptSignature(r); err == nil {
		t.Fatalf("unregistered node MUST be refused")
	}
}
