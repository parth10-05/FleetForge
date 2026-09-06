package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/parth10-05/fleetforge/internal/application/lifecycle"
	"github.com/parth10-05/fleetforge/internal/platform/buildinfo"
	"github.com/parth10-05/fleetforge/internal/platform/config"
)

func main() {
	if err := run(); err != nil {
		slog.Error("orchestrator worker stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadWorker()
	if err != nil {
		return err
	}
	controller := lifecycle.NewController()
	slog.Info("orchestrator worker started", "role", "orchestrator-worker", "environment", cfg.Environment, "version", buildinfo.Current().Version)
	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-shutdownContext.Done()
	drainContext, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if !controller.Drain(drainContext) {
		return fmt.Errorf("worker drain deadline exceeded")
	}
	return nil
}
