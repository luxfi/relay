// Package server is relayd's HTTP layer. It exposes operator-facing endpoints
// for inspecting in-flight cross-chain messages, manually triggering relay,
// and reporting health.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/luxfi/relay/pkg/relay"
)

type Config struct {
	ListenAddr string
	RelayVMRPC string
	DataDir    string
	OperatorID string
	Logger     *slog.Logger
}

type Server struct {
	cfg     Config
	mux     *http.ServeMux
	httpSrv *http.Server
	relay   *relay.Engine

	mu      sync.RWMutex
	startAt time.Time
}

func New(cfg Config) (*Server, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	eng, err := relay.New(relay.Config{
		RelayVMRPC: cfg.RelayVMRPC,
		StatePath:  filepath.Join(cfg.DataDir, "relay.state"),
		OperatorID: cfg.OperatorID,
		Logger:     cfg.Logger,
	})
	if err != nil {
		return nil, fmt.Errorf("init relay engine: %w", err)
	}

	s := &Server{cfg: cfg, mux: http.NewServeMux(), relay: eng}
	s.routes()
	s.httpSrv = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s, nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("/v1/health", s.handleHealth)
	s.mux.HandleFunc("/v1/status", s.handleStatus)
	s.mux.HandleFunc("/v1/channels", s.handleChannels)
	s.mux.HandleFunc("/v1/messages", s.handleMessages)
	s.mux.HandleFunc("/v1/relay/trigger", s.handleTrigger)
}

func (s *Server) Run(ctx context.Context) error {
	s.mu.Lock()
	s.startAt = time.Now()
	s.mu.Unlock()

	go func() {
		if err := s.relay.Run(ctx); err != nil && ctx.Err() == nil {
			s.cfg.Logger.Error("relay engine stopped", "err", err)
		}
	}()
	return s.httpSrv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.relay.Shutdown(ctx); err != nil {
		s.cfg.Logger.Warn("relay engine shutdown", "err", err)
	}
	return s.httpSrv.Shutdown(ctx)
}

// ── Handlers ────────────────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "relayd",
		"version": "1.0.0",
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	startAt := s.startAt
	s.mu.RUnlock()

	stats := s.relay.Stats()
	writeJSON(w, http.StatusOK, map[string]any{
		"operatorId":      s.cfg.OperatorID,
		"relayVmRpc":      s.cfg.RelayVMRPC,
		"uptimeSeconds":   int(time.Since(startAt).Seconds()),
		"channelsTracked": stats.ChannelsTracked,
		"messagesPending": stats.MessagesPending,
		"messagesRelayed": stats.MessagesRelayed,
		"lastError":       stats.LastError,
	})
}

func (s *Server) handleChannels(w http.ResponseWriter, _ *http.Request) {
	chs, err := s.relay.ListChannels()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": chs})
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	msgs, err := s.relay.ListMessages(state)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func (s *Server) handleTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("POST required"))
		return
	}
	var body struct{ MessageID string `json:"messageId"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.relay.Trigger(body.MessageID); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"triggered": body.MessageID})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]any{"error": err.Error()})
}
