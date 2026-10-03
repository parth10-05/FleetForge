// Command discovery-worker runs only the read-only DISCOVER work class.
// It has no artifact-store or firmware-submission dependencies.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	app "github.com/parth10-05/fleetforge/internal/application/inventory"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/buildinfo"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticdiscovery"
	"github.com/parth10-05/fleetforge/internal/redfish"
)

func main() {
	if err := run(); err != nil {
		slog.Error("discovery worker stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadDiscoveryWorker()
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
		return fmt.Errorf("configure synthetic discovery credentials: %w", err)
	}
	adapter := postgres.DiscoveryWorkAdapter{Pool: pool}
	executor := syntheticdiscovery.Executor{Loader: adapter, Client: redfish.NewClient(redfish.ClientConfig{AllowedCIDRs: cfg.AllowedCIDRs}), Resolver: resolver}
	slog.Info("discovery worker started", "role", "discovery-worker", "environment", cfg.Environment, "version", buildinfo.Current().Version)
	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := runOnce(shutdownContext, adapter, executor, adapter, cfg.Owner); err != nil {
			slog.Warn("discovery work cycle failed", "error", err.Error())
		}
		select {
		case <-shutdownContext.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func runOnce(ctx context.Context, claimer app.DiscoveryWorkClaimer, executor app.DiscoveryExecutor, writer app.DiscoveryResultWriter, owner string) error {
	now := time.Now().UTC()
	token, err := leaseToken()
	if err != nil {
		return err
	}
	request := app.DiscoveryClaimRequest{Owner: owner, LeaseToken: token, Now: now, LeaseExpiresAt: now.Add(30 * time.Second)}
	_, err = app.RunDiscoveryOnce(ctx, claimer, executor, writer, request, retryAt)
	return err
}

// retryAt is intentionally deterministic: attempt one retries after one second;
// later attempts retry after two seconds plus no more than 250ms of work-ID
// jitter. The third claimed attempt is completed by persistence instead of
// rescheduled, but still receives a valid value for atomic result recording.
func retryAt(work app.DiscoveryWork, now time.Time) time.Time {
	delay := time.Second
	if work.Attempt > 1 {
		digest := sha256.Sum256([]byte(work.ID))
		delay = 2*time.Second + time.Duration(digest[0]%251)*time.Millisecond
	}
	return now.Add(delay)
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
