// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zaptransport

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	_, p, _ := net.SplitHostPort(l.Addr().String())
	port, _ := strconv.Atoi(p)
	return port
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestZAPTransport_BroadcastReceipt exercises the end-to-end ZAP path:
// node A broadcasts a JSON-encoded receipt, node B's handler receives it.
// This is the canonical intra-Lux operator-to-operator transport.
func TestZAPTransport_BroadcastReceipt(t *testing.T) {
	portA := freePort(t)
	portB := freePort(t)

	a, err := New(Config{NodeID: "relay-a", Port: portA, NoDiscovery: true, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("new a: %v", err)
	}
	b, err := New(Config{NodeID: "relay-b", Port: portB, NoDiscovery: true, Logger: quietLogger()})
	if err != nil {
		t.Fatalf("new b: %v", err)
	}

	if err := a.Start(); err != nil {
		t.Fatalf("start a: %v", err)
	}
	defer a.Stop()
	if err := b.Start(); err != nil {
		t.Fatalf("start b: %v", err)
	}
	defer b.Stop()

	// Install receipt handler on B.
	var wg sync.WaitGroup
	wg.Add(1)
	var received []byte
	var mu sync.Mutex
	b.HandleReceipt(func(_ context.Context, _ string, payload []byte) error {
		mu.Lock()
		received = append([]byte(nil), payload...)
		mu.Unlock()
		wg.Done()
		return nil
	})

	// A connects directly to B (no mDNS in tests).
	if err := a.ConnectDirect("127.0.0.1:" + strconv.Itoa(portB)); err != nil {
		t.Fatalf("connect a->b: %v", err)
	}

	// Build a real JSON receipt payload and broadcast it.
	receipt := map[string]any{
		"scheme":    1,
		"timestamp": 1700000000,
		"sig":       "deadbeef",
	}
	body, _ := json.Marshal(receipt)

	errs := a.BroadcastReceipt(context.Background(), body)
	for peer, err := range errs {
		if err != nil {
			t.Fatalf("broadcast to %s: %v", peer, err)
		}
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for receipt")
	}

	mu.Lock()
	defer mu.Unlock()
	if string(received) != string(body) {
		t.Fatalf("receipt payload mismatch: got %q want %q", received, body)
	}
}

// TestZAPTransport_NoNodeIDRejected ensures the constructor refuses an
// empty NodeID (operator misconfiguration).
func TestZAPTransport_NoNodeIDRejected(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatalf("expected empty NodeID to be refused")
	}
}
