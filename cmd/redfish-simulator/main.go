package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/parth10-05/fleetforge/internal/delivery/healthhttp"
	"github.com/parth10-05/fleetforge/internal/platform/buildinfo"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	"github.com/parth10-05/fleetforge/internal/platform/simulatorstate"
)

func main() {
	if err := run(); err != nil {
		slog.Error("redfish simulator stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadSimulator()
	if err != nil {
		return err
	}
	if err := simulatorstate.EnsureRecoverable(cfg.StatePath, cfg.JournalPath); err != nil {
		return err
	}
	handler := healthhttp.NewHandler(healthhttp.FileProbe{Path: cfg.ReadinessFile}, cfg.ReadinessTimeout, buildinfo.Current())
	handler.MarkStarted()
	server := &http.Server{Addr: cfg.Address, Handler: handler.Routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8 << 10}
	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errorsChannel := make(chan error, 1)
	go func() { errorsChannel <- server.ListenAndServe() }()
	slog.Info("redfish simulator started", "role", "redfish-simulator", "environment", cfg.Environment, "address", cfg.Address, "version", buildinfo.Current().Version)
	select {
	case err := <-errorsChannel:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-shutdownContext.Done():
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return nil
	}
}
