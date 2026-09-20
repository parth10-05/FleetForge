//go:build postgresintegration

package postgres

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/config"
)

func TestMigrationLockSerializesConcurrentRunners(t *testing.T) {
	harness := New(t)
	settings := config.PostgreSQL{
		ApplicationDSN: harness.DSN, MigrationDSN: harness.DSN,
		MaxConns: 1, AcquireTimeout: 2 * time.Second,
		TransactionTimeout: 5 * time.Second, StatementTimeout: time.Second,
		MaxConnLifetime: time.Minute, MaxConnIdleTime: time.Minute,
	}
	first, err := persistence.OpenMigrationDB(context.Background(), settings)
	if err != nil {
		t.Fatalf("open first migration connection: %v", err)
	}
	defer first.Close()
	second, err := persistence.OpenMigrationDB(context.Background(), settings)
	if err != nil {
		t.Fatalf("open second migration connection: %v", err)
	}
	defer second.Close()

	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	errors := make(chan error, 2)
	var active, maximum atomic.Int32
	var wait sync.WaitGroup
	lockedRun := func(database *sql.DB) {
		defer wait.Done()
		errors <- persistence.WithMigrationLock(context.Background(), database, func(context.Context) error {
			count := active.Add(1)
			for previous := maximum.Load(); count > previous && !maximum.CompareAndSwap(previous, count); previous = maximum.Load() {
			}
			defer active.Add(-1)
			entered <- struct{}{}
			<-release
			return nil
		})
	}
	wait.Add(2)
	go lockedRun(first)
	go lockedRun(second)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("no migration runner acquired the advisory lock")
	}
	select {
	case <-entered:
		t.Fatal("two migration runners entered the critical section concurrently")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("migration lock runner error: %v", err)
		}
	}
	if maximum.Load() != 1 {
		t.Fatalf("maximum concurrent migration runners = %d, want 1", maximum.Load())
	}
}
