// Command migrate is the only Phase 03 migration entrypoint. It uses a direct
// PostgreSQL connection and refuses to expose DSNs in process output.
package main

import (
	"context"
	"log"
	"os"

	"github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/config"
)

func main() {
	settings, err := config.LoadPostgreSQL()
	if err != nil {
		log.Print("invalid migration configuration")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), settings.TransactionTimeout)
	defer cancel()
	if err := postgres.RunMigrations(ctx, settings, os.DirFS("db/migrations")); err != nil {
		log.Print("migration failed")
		os.Exit(1)
	}
}
