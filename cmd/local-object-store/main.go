// local-object-store serves the local-versioned direct-upload data plane.
// It is intentionally separate from the control API: artifact bytes never
// transit the control-plane listener.
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

	artifactapp "github.com/parth10-05/fleetforge/internal/application/artifacts"
	"github.com/parth10-05/fleetforge/internal/delivery/healthhttp"
	"github.com/parth10-05/fleetforge/internal/delivery/localobjecthttp"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/buildinfo"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
)

func main() {
	if err := run(); err != nil {
		slog.Error("local object store stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadObjectStore()
	if err != nil {
		return err
	}
	store, err := localartifactstore.OpenDisk(cfg.Root)
	if err != nil {
		return fmt.Errorf("open local object store: %w", err)
	}
	database, err := config.LoadPostgreSQL()
	if err != nil {
		return err
	}
	pool, err := postgres.OpenApplicationPool(context.Background(), database)
	if err != nil {
		return err
	}
	defer pool.Close()
	completion := artifactapp.NewWithCompletion(nil, nil, store, postgres.ArtifactBindingAdapter{Pool: pool}, nil)
	health := healthhttp.NewHandler(healthhttp.FileProbe{Path: cfg.ReadinessFile}, cfg.ReadinessTimeout, buildinfo.Current())
	health.MarkStarted()
	server := newServer(cfg.Address, routesWithCompletion(store, cfg.HMACKey, health, completion))
	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errorsChannel := make(chan error, 1)
	go func() { errorsChannel <- server.ListenAndServe() }()
	slog.Info("local object store started", "role", "local-object-store", "environment", cfg.Environment, "address", cfg.Address, "version", buildinfo.Current().Version)
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

func routes(store *localartifactstore.DiskStore, hmacKey []byte, health *healthhttp.Handler) http.Handler {
	return routesWithCompletion(store, hmacKey, health, nil)
}

func routesWithCompletion(store *localartifactstore.DiskStore, hmacKey []byte, health *healthhttp.Handler, completion interface {
	CompleteUpload(context.Context, string, artifact.ObjectIdentity) error
}) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/objects/upload", localobjecthttp.New(store, hmacKey, nil).WithCompletion(completion).Routes())
	mux.Handle("/", health.Routes())
	return mux
}

func newServer(address string, handler http.Handler) *http.Server {
	return &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8 << 10}
}
