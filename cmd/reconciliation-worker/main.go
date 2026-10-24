// Command reconciliation-worker resolves ambiguous firmware submissions from
// the simulator's independent retained journal. It has no firmware submit,
// artifact, or target credential capability.
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
	"github.com/parth10-05/fleetforge/internal/platform/simulatorjournal"
)

func main() {
	if err := run(); err != nil {
		slog.Error("reconciliation worker stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadReconciliationWorker()
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
	persistence := postgres.ExecutionAdapter{Pool: pool}
	worker := execution.ReconciliationWorker{Claimer: persistence, Repository: persistence, Journal: simulatorjournal.Reader{Path: cfg.JournalPath}, RunID: cfg.RunID, Now: func() time.Time { return time.Now().UTC() }}
	slog.Info("reconciliation worker started", "role", "reconciliation-worker", "environment", cfg.Environment, "version", buildinfo.Current().Version)
	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := runOnce(shutdown, worker, cfg.Owner); err != nil {
			slog.Warn("reconciliation cycle failed", "error", err.Error())
		}
		select {
		case <-shutdown.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func runOnce(ctx context.Context, worker execution.ReconciliationWorker, owner string) error {
	token, err := leaseToken()
	if err != nil {
		return err
	}
	_, err = worker.RunOnce(ctx, owner, token)
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
	return fmt.Sprintf("%s-%s-%s-%s-%s", raw[0:8], raw[8:12], raw[12:16], raw[16:20], raw[20:32]), nil
}
