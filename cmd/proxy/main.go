package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vectorcore/gtp_proxy/internal/api"
	"github.com/vectorcore/gtp_proxy/internal/config"
	"github.com/vectorcore/gtp_proxy/internal/gtpc"
	"github.com/vectorcore/gtp_proxy/internal/gtpu"
	"github.com/vectorcore/gtp_proxy/internal/logging"
	"github.com/vectorcore/gtp_proxy/internal/metrics"
	"github.com/vectorcore/gtp_proxy/internal/session"
)

var version = "dev"

func main() {
	cfgPath := flag.String("c", "config.yaml", "path to config file")
	flag.Parse()

	manager, err := config.LoadManager(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}
	defer manager.Close()

	logger, err := logging.New(manager.Snapshot().Log.Level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup logging: %v\n", err)
		os.Exit(1)
	}
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sessions := session.NewTable()
	registry := metrics.New()
	gtpcServer := gtpc.NewServer(manager, sessions, registry, logger)
	gtpuServer := gtpu.NewServer(manager, sessions, registry, logger)
	apiServer := api.New(manager, sessions, registry, version, logger)
	cleanupCh := session.StartCleanupLoop(ctx, sessions, manager.Snapshot().Proxy.Timeouts.CleanupIntervalDuration(), logger)
	go func() {
		for n := range cleanupCh {
			registry.AddSessionTimeoutDeletes(n)
		}
	}()

	errCh := make(chan error, 3)
	go func() { errCh <- gtpcServer.Start(ctx) }()
	go func() { errCh <- gtpuServer.Start(ctx) }()
	go func() { errCh <- apiServer.Start(ctx, manager.Snapshot().API.Listen) }()

	select {
	case <-ctx.Done():
		return
	case err := <-errCh:
		if err != nil {
			slog.Error("service stopped", "err", err)
			os.Exit(1)
		}
	}
}
