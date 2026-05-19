// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vm

import (
	"encoding/json"
	"testing"
)

// FuzzReceiptDecode feeds the JSON receipt decoder arbitrary bytes to confirm
// it never panics — the decoder is at the trust boundary between R-Chain RPC
// callers and the receipt registry.
func FuzzReceiptDecode(f *testing.F) {
	// Seed with a known-good receipt shape.
	good := &SignedReceipt{
		Scheme:    0x01,
		Timestamp: 1,
		Signature: []byte("seed-sig"),
	}
	if b, err := json.Marshal(good); err == nil {
		f.Add(b)
	}
	// Edge cases.
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"scheme":255,"signature":null}`))
	f.Add([]byte(`{"scheme":1,"timestamp":-1}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var r SignedReceipt
		_ = json.Unmarshal(data, &r) // must not panic
	})
}

// FuzzMessageDecode does the same for the cross-chain Message envelope.
func FuzzMessageDecode(f *testing.F) {
	good := &Message{
		Sequence: 1,
		State:    MessagePending,
		Payload:  []byte("x"),
	}
	if b, err := json.Marshal(good); err == nil {
		f.Add(b)
	}
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"sequence":-1}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var m Message
		_ = json.Unmarshal(data, &m) // must not panic
	})
}
