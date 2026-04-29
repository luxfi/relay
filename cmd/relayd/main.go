// Package main runs `relayd` — the Lux cross-chain relay operator daemon.
//
// `relayd` is a standalone operator process (no luxd validator required).
// It reads source-chain logs (e.g. SecuritiesGateway.Outbound), packages
// them into R-Chain (relayvm) channel messages, and forwards verified
// messages from R-Chain to destination chains. For non-EVM destinations
// (OP_NET, Bitcoin), it hands off to the FROST/Taproot signer in
// `luxfi/mpc` instead of submitting directly.
//
// Operationally the daemon is identical in shape to `mpcd` and `kms`:
// HTTP server, NATS subscription, persistent state — process-per-operator.
//
// The chain-side (R-Chain consensus on `chains/relayvm`) is the source of
// truth for verified message acceptance. `relayd` is just the courier.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/luxfi/relay/pkg/server"
)

const (
	defaultListenAddr = ":7700"
	defaultRelayVMRPC = "http://127.0.0.1:9650/ext/bc/R/rpc"
	shutdownTimeout   = 15 * time.Second
)

func main() {
	var (
		listenAddr = flag.String("listen", env("RELAYD_LISTEN", defaultListenAddr), "HTTP listen address")
		relayVMRPC = flag.String("relayvm-rpc", env("RELAYD_RELAYVM_RPC", defaultRelayVMRPC), "R-Chain (relayvm) JSON-RPC URL")
		dataDir    = flag.String("data-dir", env("RELAYD_DATA_DIR", "data"), "persistent state directory")
		operatorID = flag.String("operator-id", env("RELAYD_OPERATOR_ID", ""), "this operator's NodeID (hex/CB58)")
		logLevel   = flag.String("log-level", env("RELAYD_LOG_LEVEL", "info"), "debug|info|warn|error")
		showVer    = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("relayd/1.0.0")
		return
	}

	logger := newLogger(*logLevel)

	if *operatorID == "" {
		logger.Error("operator-id is required (set RELAYD_OPERATOR_ID or pass --operator-id)")
		os.Exit(2)
	}

	cfg := server.Config{
		ListenAddr: *listenAddr,
		RelayVMRPC: *relayVMRPC,
		DataDir:    *dataDir,
		OperatorID: *operatorID,
		Logger:     logger,
	}
	srv, err := server.New(cfg)
	if err != nil {
		logger.Error("server init", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		logger.Info("relayd starting", "listen", *listenAddr, "relayvm", *relayVMRPC, "operator", *operatorID)
		if err := srv.Run(ctx); err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped with error", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	shutdownCtx, shCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
	}
	logger.Info("relayd stopped")
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
