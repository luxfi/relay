package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestServer constructs a *Server backed by a real (but quiescent)
// relay.Engine. The engine is never Run, so no outbound RPC fires; the
// HTTP handlers just exercise the in-process Stats/ListChannels/ListMessages
// surface. ZAP listener is disabled (ZAPPort=0).
func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{
		ListenAddr: "127.0.0.1:0",
		ZAPPort:    0,
		RelayVMRPC: "http://127.0.0.1:1", // unreachable but non-empty
		DataDir:    dir,
		OperatorID: "NodeID-test",
		Logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, dir
}

func httpDo(s *Server, method, target string, body io.Reader) (*http.Response, []byte) {
	req := httptest.NewRequest(method, target, body)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	resp := rec.Result()
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, b
}

// ─── Server construction ────────────────────────────────────────────────────

func TestNew_RequiresRelayVMRPC(t *testing.T) {
	_, err := New(Config{
		ListenAddr: ":0",
		DataDir:    t.TempDir(),
		OperatorID: "n",
		Logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "relayvm rpc") {
		t.Fatalf("expected error to mention relayvm rpc, got %v", err)
	}
}

func TestNew_CreatesDataDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "nested", "deep")
	_, err := New(Config{
		ListenAddr: ":0",
		RelayVMRPC: "http://127.0.0.1:1",
		DataDir:    dir,
		OperatorID: "n",
		Logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("data dir not created: err=%v info=%+v", err, info)
	}
}

func TestNew_DefaultsLoggerWhenNil(t *testing.T) {
	// New must not panic when Logger is nil — it falls back to slog JSON on stderr.
	s, err := New(Config{
		ListenAddr: ":0",
		RelayVMRPC: "http://127.0.0.1:1",
		DataDir:    t.TempDir(),
		OperatorID: "n",
		Logger:     nil,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.cfg.Logger == nil {
		t.Fatal("expected fallback logger, got nil")
	}
}

// ─── handleHealth ───────────────────────────────────────────────────────────

func TestHandleHealth_OK(t *testing.T) {
	s, _ := newTestServer(t)
	resp, body := httpDo(s, http.MethodGet, "/v1/health", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type=%q want application/json", ct)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if got["ok"] != true {
		t.Fatalf("ok=%v want true", got["ok"])
	}
	if got["service"] != "relayd" {
		t.Fatalf("service=%v want relayd", got["service"])
	}
}

// ─── handleStatus ───────────────────────────────────────────────────────────

func TestHandleStatus_ShapeAndIdentity(t *testing.T) {
	s, _ := newTestServer(t)
	resp, body := httpDo(s, http.MethodGet, "/v1/status", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if got["operatorId"] != "NodeID-test" {
		t.Fatalf("operatorId=%v", got["operatorId"])
	}
	if got["relayVmRpc"] != "http://127.0.0.1:1" {
		t.Fatalf("relayVmRpc=%v", got["relayVmRpc"])
	}
	for _, k := range []string{"uptimeSeconds", "channelsTracked", "messagesPending", "messagesRelayed"} {
		if _, ok := got[k]; !ok {
			t.Fatalf("missing key %q in status: %v", k, got)
		}
	}
}

// ─── handleZAPPeers ─────────────────────────────────────────────────────────

func TestHandleZAPPeers_DisabledWhenPortZero(t *testing.T) {
	s, _ := newTestServer(t)
	resp, body := httpDo(s, http.MethodGet, "/v1/zap/peers", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if got["enabled"] != false {
		t.Fatalf("enabled=%v want false", got["enabled"])
	}
	peers, ok := got["peers"].([]any)
	if !ok {
		t.Fatalf("peers should be a slice, got %T", got["peers"])
	}
	if len(peers) != 0 {
		t.Fatalf("peers should be empty when disabled, got %v", peers)
	}
}

// ─── handleTrigger ──────────────────────────────────────────────────────────

func TestHandleTrigger_RejectsNonPOST(t *testing.T) {
	s, _ := newTestServer(t)
	resp, _ := httpDo(s, http.MethodGet, "/v1/relay/trigger", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want 405", resp.StatusCode)
	}
}

func TestHandleTrigger_RejectsMalformedJSON(t *testing.T) {
	s, _ := newTestServer(t)
	resp, body := httpDo(s, http.MethodPost, "/v1/relay/trigger", strings.NewReader("not-json"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", resp.StatusCode, body)
	}
}

func TestHandleTrigger_PropagatesEngineError(t *testing.T) {
	// The engine has no message with this ID, so Trigger returns an error
	// which the handler maps to 502 Bad Gateway.
	s, _ := newTestServer(t)
	payload := bytes.NewBufferString(`{"messageId":"not-a-real-message"}`)
	resp, body := httpDo(s, http.MethodPost, "/v1/relay/trigger", payload)
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status=%d want 502 body=%s", resp.StatusCode, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if _, ok := got["error"]; !ok {
		t.Fatalf("expected error field in body, got %v", got)
	}
}

// ─── handleChannels / handleMessages ────────────────────────────────────────

func TestHandleChannels_OKWithEmptyEngine(t *testing.T) {
	s, _ := newTestServer(t)
	resp, body := httpDo(s, http.MethodGet, "/v1/channels", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if _, ok := got["channels"]; !ok {
		t.Fatalf("missing channels key: %v", got)
	}
}

func TestHandleMessages_AcceptsStateQuery(t *testing.T) {
	s, _ := newTestServer(t)
	resp, body := httpDo(s, http.MethodGet, "/v1/messages?state=pending", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if _, ok := got["messages"]; !ok {
		t.Fatalf("missing messages key: %v", got)
	}
}

// ─── Routes wiring ──────────────────────────────────────────────────────────

func TestRoutes_AllPathsRegistered(t *testing.T) {
	s, _ := newTestServer(t)
	for _, path := range []string{
		"/v1/health", "/v1/status", "/v1/channels",
		"/v1/messages", "/v1/zap/peers",
	} {
		resp, _ := httpDo(s, http.MethodGet, path, nil)
		if resp.StatusCode == http.StatusNotFound {
			t.Errorf("route %s not registered", path)
		}
	}
	// trigger is POST-only; GET returns 405 (still registered).
	resp, _ := httpDo(s, http.MethodGet, "/v1/relay/trigger", nil)
	if resp.StatusCode == http.StatusNotFound {
		t.Errorf("route /v1/relay/trigger not registered")
	}
}

// ─── Shutdown ───────────────────────────────────────────────────────────────

func TestShutdown_NoListenerStarted_DoesNotError(t *testing.T) {
	s, _ := newTestServer(t)
	// Run() was never called; Shutdown must still complete cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Shutdown(ctx); err != nil && err != http.ErrServerClosed && err != context.Canceled {
		t.Fatalf("unexpected shutdown error: %v", err)
	}
}
