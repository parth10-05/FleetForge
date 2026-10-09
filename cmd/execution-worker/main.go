// Command execution-worker performs only the one-shot SUBMIT work class.
// Observation and reconciliation are separate durable transitions and cannot
// be retried through this target-side process.
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
	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
	"github.com/parth10-05/fleetforge/internal/platform/redfishexecution"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
	"github.com/parth10-05/fleetforge/internal/redfish"
)

func main() {
	if err := run(); err != nil {
		slog.Error("execution worker stopped", "error", err.Error())
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
	store, err := localartifactstore.OpenDisk(cfg.Root)
	if err != nil {
		return fmt.Errorf("open execution artifact store: %w", err)
	}
	resolver, err := syntheticcredentials.New(cfg.Credentials)
	if err != nil {
		return fmt.Errorf("configure execution credentials: %w", err)
	}
	persistence := postgres.ExecutionAdapter{Pool: pool}
	target := redfishexecution.Adapter{Loader: redfishexecution.PostgresTargetLoader{Pool: pool}, Client: redfish.NewClient(redfish.ClientConfig{AllowedCIDRs: cfg.AllowedCIDRs}), Resolver: resolver}
	worker := execution.SubmitWorker{Claimer: persistence, Repository: persistence, Artifacts: localartifactstore.ExecutionReader{Store: store}, Target: target, ActionPath: "UpdateService/update"}
	slog.Info("execution worker started", "role", "execution-worker", "environment", cfg.Environment, "version", buildinfo.Current().Version)
	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := runOnce(shutdown, worker, cfg.Owner); err != nil {
			slog.Warn("firmware submit cycle failed", "error", err.Error())
		}
		select {
		case <-shutdown.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func runOnce(ctx context.Context, worker execution.SubmitWorker, owner string) error {
	token, err := leaseToken()
	if err != nil {
		return err
	}
	_, err = worker.RunOnce(ctx, owner, token)
	return err
}
func leaseToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	raw := hex.EncodeToString(bytes)
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32], nil
}
