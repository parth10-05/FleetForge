package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	artifactapp "github.com/parth10-05/fleetforge/internal/application/artifacts"
	"github.com/parth10-05/fleetforge/internal/application/lifecycle"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/buildinfo"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
)

func main() {
	if err := run(); err != nil {
		slog.Error("orchestrator worker stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadArtifactWorker()
	if err != nil {
		return err
	}
	store, err := localartifactstore.OpenDisk(cfg.Root)
	if err != nil {
		return fmt.Errorf("open artifact store: %w", err)
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
	verification := postgres.ArtifactVerificationAdapter{Pool: pool}
	service := artifactapp.NewWithCompletion(nil, nil, store, nil, nil).WithVerification(verification, verification)
	work := postgres.ArtifactVerificationWorkAdapter{Pool: pool}
	worker := artifactapp.NewWorker(service, work, work, nil)
	controller := lifecycle.NewController()
	slog.Info("orchestrator worker started", "role", "orchestrator-worker", "environment", cfg.Environment, "version", buildinfo.Current().Version)
	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := runVerificationOnce(shutdownContext, worker, cfg.Owner); err != nil {
			slog.Warn("artifact verification work cycle failed", "error", err.Error())
		}
		select {
		case <-shutdownContext.Done():
			goto shutdown
		case <-ticker.C:
		}
	}

shutdown:
	drainContext, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if !controller.Drain(drainContext) {
		return fmt.Errorf("worker drain deadline exceeded")
	}
	return nil
}

func runVerificationOnce(ctx context.Context, worker *artifactapp.Worker, owner string) error {
	now := time.Now().UTC()
	token, err := leaseToken()
	if err != nil {
		return err
	}
	return worker.RunOnce(ctx, artifactapp.VerificationWorkClaimRequest{Owner: owner, LeaseToken: token, Now: now, LeaseExpiresAt: now.Add(time.Minute)})
}

func leaseToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	hexadecimal := hex.EncodeToString(bytes)
	return hexadecimal[0:8] + "-" + hexadecimal[8:12] + "-" + hexadecimal[12:16] + "-" + hexadecimal[16:20] + "-" + hexadecimal[20:32], nil
}
