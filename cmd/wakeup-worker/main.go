// Command wakeup-worker composes the bounded P4 local publisher, queue, and
// safe worker. It executes only SAFETY_PROBE; it has no firmware client.
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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/execution"
	"github.com/parth10-05/fleetforge/internal/application/orchestration"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	"github.com/parth10-05/fleetforge/internal/platform/localwakeup"
)

func main() {
	if err := run(); err != nil {
		slog.Error("wakeup worker stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	budget, err := config.LoadOrchestrationWorker()
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
	queue, err := localwakeup.New(localwakeup.Config{Capacity: int(budget.LocalQueueCapacity), Clock: orchestration.ClockFunc(func() time.Time { return time.Now().UTC() })})
	if err != nil {
		return err
	}
	adapter := postgres.WakeupWorkAdapter{Pool: pool, Owner: budget.Owner, LeaseDuration: budget.LeaseDuration, Now: func() time.Time { return time.Now().UTC() }, NewToken: newToken}
	handlers, err := orchestration.NewMultiplexHandler([]orchestration.WorkHandler{orchestration.SafetyProbeHandler{}}, []orchestration.FinalizingWorkHandler{execution.WakeupPrecheckStartHandler{Repository: postgres.ExecutionAdapter{Pool: pool}}})
	if err != nil {
		return err
	}
	worker, err := orchestration.NewWorker(queue, queue, adapter, handlers, orchestration.WorkerConfig{ReceiveBatch: int(budget.ReceiveBatch), Concurrency: int(budget.WorkerConcurrency), HandlerTimeout: budget.HandlerTimeout})
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer queue.Close()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := cycle(ctx, pool, queue, worker, budget); err != nil {
			slog.Warn("wakeup cycle failed", "error", err.Error())
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func cycle(ctx context.Context, pool *pgxpool.Pool, queue orchestration.WakeupPublisher, worker *orchestration.Worker, budget config.OrchestrationWorker) error {
	now := time.Now().UTC()
	tokens := make([]string, budget.PublisherBatch)
	for i := range tokens {
		token, err := newToken()
		if err != nil {
			return err
		}
		tokens[i] = token
	}
	if err := postgres.PublishClaimedWakeups(ctx, pool, queue, postgres.ClaimOutboxRequest{Owner: budget.Owner, Now: now, LeaseExpiresAt: now.Add(budget.PublisherLease), Tokens: tokens, WorkTypes: []string{"CANARY_START", "LATER_WAVE_START"}}, int64(budget.RetryAttempts), func(item postgres.ClaimedWakeup, at time.Time) time.Time {
		next, err := orchestration.RetryAt(at, item.Envelope.CommandID, item.Attempts, time.Second, budget.MaxBackoff, "fleetforge.wakeup.retry/v1")
		if err != nil {
			return at.Add(time.Second)
		}
		return next
	}); err != nil {
		return fmt.Errorf("publish wakeups: %w", err)
	}
	if err := worker.RunOnce(ctx); err != nil {
		return fmt.Errorf("process wakeups: %w", err)
	}
	return nil
}

func newToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	hexadecimal := hex.EncodeToString(bytes)
	return hexadecimal[0:8] + "-" + hexadecimal[8:12] + "-" + hexadecimal[12:16] + "-" + hexadecimal[16:20] + "-" + hexadecimal[20:32], nil
}
