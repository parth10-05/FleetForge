// Command wakeup-sweeper runs one bounded, database-led wakeup repair pass.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/config"
)

func main() {
	if err := run(context.Background()); err != nil {
		slog.Error("wakeup sweeper stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	budget, err := config.LoadOrchestrationWorker()
	if err != nil {
		return err
	}
	database, err := config.LoadPostgreSQL()
	if err != nil {
		return err
	}
	pool, err := postgres.OpenApplicationPool(ctx, database)
	if err != nil {
		return err
	}
	defer pool.Close()
	now := time.Now().UTC()
	if _, err := postgres.RepairExpiredOutbox(ctx, pool, now, int(budget.RepairBatch)); err != nil {
		return fmt.Errorf("repair expired outbox: %w", err)
	}
	if _, err := postgres.RepairExpiredDueWork(ctx, pool, now, int(budget.RepairBatch)); err != nil {
		return fmt.Errorf("repair expired due work: %w", err)
	}
	if _, err := postgres.RepairMissingWakeups(ctx, pool, postgres.RepairWakeupRequest{Now: now, DeliveryGrace: budget.RepairGrace, Limit: int(budget.RepairBatch), MaxGeneration: int64(budget.RetryAttempts)}); err != nil {
		return fmt.Errorf("repair missing wakeups: %w", err)
	}
	telemetry, err := postgres.CollectWakeupTelemetry(ctx, pool, now)
	if err != nil {
		return fmt.Errorf("collect wakeup telemetry: %w", err)
	}
	slog.Info("wakeup sweeper completed", "repair_batch", budget.RepairBatch, "pending_wakeups", telemetry.PendingWakeupCount, "pending_due_work", telemetry.PendingDueWorkCount, "open_quarantines", telemetry.OpenQuarantineCount)
	return nil
}
