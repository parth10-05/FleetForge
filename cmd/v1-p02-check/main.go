// Command v1-p02-check verifies that the configured PostgreSQL application
// connection can support the P02 persistence boundary. It is read-only and
// intentionally does not start workers, issue upload grants, or contact BMCs.
package main

import (
	"context"
	"log"
	"os"

	"github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/config"
)

func main() {
	if err := run(); err != nil {
		log.Print("v1-p02 check failed")
		os.Exit(1)
	}
	log.Print("v1-p02 database readiness check passed")
}

func run() error {
	settings, err := config.LoadPostgreSQL()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), settings.AcquireTimeout)
	defer cancel()
	pool, err := postgres.OpenApplicationPool(ctx, settings)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return err
	}
	var migrationTable bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('goose_db_version') IS NOT NULL`).Scan(&migrationTable); err != nil {
		return err
	}
	if !migrationTable {
		return errP02SchemaUnavailable{}
	}
	return nil
}

type errP02SchemaUnavailable struct{}

func (errP02SchemaUnavailable) Error() string { return "P02 schema is unavailable" }
