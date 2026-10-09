// Command observation-worker processes only task, inventory, and health reads.
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

	"github.com/parth10-05/fleetforge/internal/application/execution"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/buildinfo"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	"github.com/parth10-05/fleetforge/internal/platform/redfishexecution"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
	"github.com/parth10-05/fleetforge/internal/redfish"
)

func main() {
	if err := run(); err != nil {
		slog.Error("observation worker stopped", "error", err.Error())
		os.Exit(1)
	}
}
func run() error {
	cfg, err := config.LoadExecutionWorker()
	if err != nil {
		return err
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
	resolver, err := syntheticcredentials.New(cfg.Credentials)
	if err != nil {
		return fmt.Errorf("configure observation credentials: %w", err)
	}
	persistence := postgres.ExecutionAdapter{Pool: pool}
	target := redfishexecution.Adapter{Loader: redfishexecution.PostgresTargetLoader{Pool: pool}, Client: redfish.NewClient(redfish.ClientConfig{AllowedCIDRs: cfg.AllowedCIDRs}), Resolver: resolver}
	worker := execution.ObservationWorker{Claimer: persistence, Repository: persistence, Target: target}
	slog.Info("observation worker started", "role", "observation-worker", "environment", cfg.Environment, "version", buildinfo.Current().Version)
	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := runOnce(shutdown, worker, cfg.Owner); err != nil {
			slog.Warn("observation cycle failed", "error", err.Error())
		}
		select {
		case <-shutdown.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func runOnce(ctx context.Context, worker execution.ObservationWorker, owner string) error {
	token, err := leaseToken()
	if err != nil {
		return err
	}
	_, err = worker.RunOnce(ctx, owner+"-observer", token)
	return err
}
func leaseToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	raw := hex.EncodeToString(b)
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32], nil
}
