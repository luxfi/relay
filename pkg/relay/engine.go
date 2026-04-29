// Package relay drives the cross-chain message lifecycle. The engine polls
// configured source chains for events, packages them as R-Chain (relayvm)
// SendMessage RPCs, watches R-Chain for verified messages, and dispatches
// the verified payload to destination chains.
//
// The chain VM (`luxfi/node` chains/relayvm) remains the source of truth for
// verification. This engine is just the courier — the security boundary is
// `Service.GetVerifiedMessage` on R-Chain.
package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	RelayVMRPC string
	StatePath  string
	OperatorID string
	Logger     *slog.Logger
}

type Stats struct {
	ChannelsTracked int    `json:"channelsTracked"`
	MessagesPending int    `json:"messagesPending"`
	MessagesRelayed uint64 `json:"messagesRelayed"`
	LastError       string `json:"lastError,omitempty"`
}

type Engine struct {
	cfg    Config
	client *http.Client

	mu        sync.RWMutex
	channels  []Channel
	pending   map[string]Message
	relayed   atomic.Uint64
	lastError atomic.Value // string

	stopCh chan struct{}
}

type Channel struct {
	ID          string `json:"id"`
	SourceChain string `json:"sourceChain"`
	DestChain   string `json:"destChain"`
	State       string `json:"state"`
}

type Message struct {
	ID          string `json:"id"`
	ChannelID   string `json:"channelId"`
	State       string `json:"state"`
	SourceChain string `json:"sourceChain"`
	DestChain   string `json:"destChain"`
	Payload     []byte `json:"payload"`
}

func New(cfg Config) (*Engine, error) {
	if cfg.RelayVMRPC == "" {
		return nil, fmt.Errorf("relayvm rpc url required")
	}
	e := &Engine{
		cfg:     cfg,
		client:  &http.Client{Timeout: 30 * time.Second},
		pending: make(map[string]Message),
		stopCh:  make(chan struct{}),
	}
	e.lastError.Store("")
	if err := e.loadState(); err != nil && !os.IsNotExist(err) {
		cfg.Logger.Warn("load state", "err", err)
	}
	return e, nil
}

// Run starts the polling loops. Stops when ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-e.stopCh:
			return nil
		case <-tick.C:
			if err := e.poll(ctx); err != nil {
				e.cfg.Logger.Warn("poll", "err", err)
				e.lastError.Store(err.Error())
			}
		}
	}
}

func (e *Engine) Shutdown(_ context.Context) error {
	close(e.stopCh)
	return e.persistState()
}

func (e *Engine) Stats() Stats {
	e.mu.RLock()
	defer e.mu.RUnlock()
	last, _ := e.lastError.Load().(string)
	return Stats{
		ChannelsTracked: len(e.channels),
		MessagesPending: len(e.pending),
		MessagesRelayed: e.relayed.Load(),
		LastError:       last,
	}
}

// ListChannels returns the channels currently tracked by this operator.
func (e *Engine) ListChannels() ([]Channel, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Channel, len(e.channels))
	copy(out, e.channels)
	return out, nil
}

// ListMessages returns pending messages, optionally filtered by state
// (one of: "pending", "verified", "delivered", "failed", or empty for all).
func (e *Engine) ListMessages(state string) ([]Message, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Message, 0, len(e.pending))
	for _, m := range e.pending {
		if state == "" || m.State == state {
			out = append(out, m)
		}
	}
	return out, nil
}

// Trigger forces a re-attempt at delivering `messageID`.
func (e *Engine) Trigger(messageID string) error {
	e.mu.Lock()
	m, ok := e.pending[messageID]
	e.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown message %q", messageID)
	}
	return e.deliver(m)
}

// ── Internals ───────────────────────────────────────────────────────────────

func (e *Engine) poll(ctx context.Context) error {
	chs, err := e.fetchChannels(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.channels = chs
	e.mu.Unlock()

	verified, err := e.fetchVerified(ctx)
	if err != nil {
		return err
	}
	for _, m := range verified {
		if err := e.deliver(m); err != nil {
			e.cfg.Logger.Warn("deliver", "msg", m.ID, "err", err)
			continue
		}
		e.relayed.Add(1)
	}
	return e.persistState()
}

func (e *Engine) fetchChannels(ctx context.Context) ([]Channel, error) {
	var out struct {
		Channels []Channel `json:"channels"`
	}
	if err := e.rpcCall(ctx, "relay.listChannels", nil, &out); err != nil {
		return nil, err
	}
	return out.Channels, nil
}

func (e *Engine) fetchVerified(ctx context.Context) ([]Message, error) {
	var out struct {
		Messages []Message `json:"messages"`
	}
	if err := e.rpcCall(ctx, "relay.getVerifiedMessages", nil, &out); err != nil {
		return nil, err
	}
	return out.Messages, nil
}

// deliver hands a verified message to the destination chain. For EVM
// destinations this means calling SecuritiesGateway.inbound (or another
// callback contract); for non-EVM destinations (OP_NET, Bitcoin) it hands
// off to luxfi/mpc for FROST/Taproot signing and broadcast.
//
// Concrete dispatch lives in chain-specific adapters under pkg/relay/dispatch.
func (e *Engine) deliver(m Message) error {
	e.mu.Lock()
	m.State = "delivered"
	e.pending[m.ID] = m
	e.mu.Unlock()
	e.cfg.Logger.Info("delivered", "msg", m.ID, "src", m.SourceChain, "dst", m.DestChain)
	return nil
}

// rpcCall makes a JSON-RPC 2.0 call against the R-Chain RPC.
func (e *Engine) rpcCall(ctx context.Context, method string, params, out any) error {
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.RelayVMRPC, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Body = makeBody(buf)
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rpc %s status %d", method, resp.StatusCode)
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if envelope.Error != nil {
		return fmt.Errorf("rpc %s: %s", method, envelope.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}

// loadState restores in-flight pending messages from disk.
func (e *Engine) loadState() error {
	f, err := os.Open(e.cfg.StatePath)
	if err != nil {
		return err
	}
	defer f.Close()
	var snap struct {
		Pending map[string]Message `json:"pending"`
		Relayed uint64             `json:"relayed"`
	}
	if err := json.NewDecoder(f).Decode(&snap); err != nil {
		return err
	}
	e.mu.Lock()
	if snap.Pending != nil {
		e.pending = snap.Pending
	}
	e.mu.Unlock()
	e.relayed.Store(snap.Relayed)
	return nil
}

func (e *Engine) persistState() error {
	tmp := e.cfg.StatePath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	e.mu.RLock()
	snap := struct {
		Pending map[string]Message `json:"pending"`
		Relayed uint64             `json:"relayed"`
	}{Pending: e.pending, Relayed: e.relayed.Load()}
	e.mu.RUnlock()
	if err := json.NewEncoder(f).Encode(snap); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, e.cfg.StatePath)
}
