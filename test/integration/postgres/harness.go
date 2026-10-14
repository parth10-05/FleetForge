// Package postgres provides CI-only real PostgreSQL integration tests. The
// shared harness has no init-time connection, so normal unit commands stay
// database-free.
package postgres

import (
	"testing"

	testpostgres "github.com/parth10-05/fleetforge/test/support/postgres"
)

const TestDSNEnvironment = testpostgres.TestDSNEnvironment

type Harness = testpostgres.Harness

func New(t testing.TB) Harness { return testpostgres.New(t) }
