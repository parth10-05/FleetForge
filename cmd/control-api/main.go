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

	"github.com/parth10-05/fleetforge/internal/application/control"
	"github.com/parth10-05/fleetforge/internal/delivery/controlhttp"
	"github.com/parth10-05/fleetforge/internal/delivery/eventhinthttp"
	"github.com/parth10-05/fleetforge/internal/delivery/healthhttp"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/buildinfo"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	"github.com/parth10-05/fleetforge/internal/platform/localoidc"
)

func main() {
	if err := run(); err != nil {
		slog.Error("control API stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadAPI()
	if err != nil {
		return err
	}
	settings, err := config.LoadPostgreSQL()
	if err != nil {
		return err
	}
	pool, err := postgres.OpenApplicationPool(context.Background(), settings)
	if err != nil {
		return err
	}
	defer pool.Close()
	health := healthhttp.NewHandler(healthhttp.FileProbe{Path: cfg.ReadinessFile}, cfg.ReadinessTimeout, buildinfo.Current())
	health.MarkStarted()
	verifier := localoidc.Verifier{Issuer: cfg.LocalIdentity.Issuer, Audience: cfg.LocalIdentity.Audience, Key: cfg.LocalIdentity.Key, RecentAuth: cfg.LocalIdentity.RecentAuth, Now: time.Now}
	mux := http.NewServeMux()
	mux.Handle("/", health.Routes())
	mux.Handle("/v1/", controlhttp.NewRouterWithResources(verifier, time.Now, control.NewTargetService(postgres.ControlTargetAdapter{Pool: pool}, time.Now), control.NewArtifactService(postgres.ControlArtifactAdapter{Pool: pool}, time.Now), cfg.CursorHMACKey))
	mux.Handle("/v1/events/synthetic", eventhinthttp.New(cfg.EventHMACKey, postgres.EventHintStore{Pool: pool}, time.Now).Routes())
	server := newServer(cfg.Address, mux)
	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errorsChannel := make(chan error, 1)
	go func() { errorsChannel <- server.ListenAndServe() }()
	slog.Info("control API started", "role", "control-api", "environment", cfg.Environment, "address", cfg.Address, "version", buildinfo.Current().Version)
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

func newServer(address string, handler http.Handler) *http.Server {
	return &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8 << 10}
}
