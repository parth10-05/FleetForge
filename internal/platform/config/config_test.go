package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadValidRoleConfigurations(t *testing.T) {
	t.Setenv("FLEETFORGE_ENV", "test")
	t.Setenv("FLEETFORGE_API_ADDR", "127.0.0.1:8080")
	t.Setenv("FLEETFORGE_SIMULATOR_ADDR", "127.0.0.1:8081")
	t.Setenv("FLEETFORGE_SIMULATOR_STATE_PATH", "state.json")
	t.Setenv("FLEETFORGE_SIMULATOR_JOURNAL_PATH", "journal.json")
	if _, err := LoadAPI(); err != nil {
		t.Fatalf("LoadAPI() error = %v", err)
	}
	if _, err := LoadWorker(); err != nil {
		t.Fatalf("LoadWorker() error = %v", err)
	}
	if _, err := LoadSimulator(); err != nil {
		t.Fatalf("LoadSimulator() error = %v", err)
	}
}

func TestInvalidConfigurationDoesNotLeakSecret(t *testing.T) {
	const secret = "credential-that-must-not-appear"
	t.Setenv("FLEETFORGE_API_ADDR", secret)
	_, err := LoadAPI()
	if err == nil {
		t.Fatal("LoadAPI() error = nil, want invalid address error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("LoadAPI() error leaked secret: %v", err)
	}
}

func TestProductionDebugEndpointIsRejected(t *testing.T) {
	t.Setenv("FLEETFORGE_API_ADDR", "127.0.0.1:8080")
	t.Setenv("FLEETFORGE_ENV", "production")
	t.Setenv("FLEETFORGE_DEBUG_ENDPOINTS", "true")
	_, err := LoadAPI()
	if err == nil || !strings.Contains(err.Error(), "FLEETFORGE_DEBUG_ENDPOINTS") {
		t.Fatalf("LoadAPI() error = %v, want production debug endpoint error", err)
	}
}

func TestMissingRequiredConfig(t *testing.T) {
	previous, existed := os.LookupEnv(apiAddressKey)
	os.Unsetenv(apiAddressKey)
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(apiAddressKey, previous)
		} else {
			os.Unsetenv(apiAddressKey)
		}
	})
	_, err := LoadAPI()
	if err == nil || !strings.Contains(err.Error(), apiAddressKey) {
		t.Fatalf("LoadAPI() error = %v, want missing %s", err, apiAddressKey)
	}
}

func TestLoadPostgreSQLUsesBoundedRoleSpecificConfiguration(t *testing.T) {
	t.Setenv(postgresApplicationDSNKey, "postgres://app:secret@127.0.0.1:5432/fleetforge?sslmode=disable")
	t.Setenv(postgresMigrationDSNKey, "postgres://migrate:secret@127.0.0.1:5432/fleetforge?sslmode=disable")
	t.Setenv("FLEETFORGE_POSTGRES_MAX_CONNS", "7")
	t.Setenv("FLEETFORGE_POSTGRES_MIN_CONNS", "2")
	loaded, err := LoadPostgreSQL()
	if err != nil {
		t.Fatalf("LoadPostgreSQL() error = %v", err)
	}
	if loaded.MaxConns != 7 || loaded.MinConns != 2 || loaded.StatementTimeout <= 0 {
		t.Fatalf("LoadPostgreSQL() = %#v, want bounded connection settings", loaded)
	}
}

func TestLoadPostgreSQLRedactsInvalidDSN(t *testing.T) {
	const secret = "postgres://app:credential-that-must-not-appear@/bad"
	t.Setenv(postgresApplicationDSNKey, secret)
	t.Setenv(postgresMigrationDSNKey, "postgres://migrate:secret@127.0.0.1:5432/fleetforge")
	_, err := LoadPostgreSQL()
	if err == nil || !strings.Contains(err.Error(), postgresApplicationDSNKey) || strings.Contains(err.Error(), secret) {
		t.Fatalf("LoadPostgreSQL() error = %v, want safely redacted validation error", err)
	}
}
