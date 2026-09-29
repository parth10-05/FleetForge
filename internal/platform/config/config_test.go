package config

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func TestLoadValidRoleConfigurations(t *testing.T) {
	t.Setenv("FLEETFORGE_ENV", "test")
	t.Setenv("FLEETFORGE_API_ADDR", "127.0.0.1:8080")
	t.Setenv("FLEETFORGE_SIMULATOR_ADDR", "127.0.0.1:8081")
	t.Setenv("FLEETFORGE_SIMULATOR_RUN_ROOT", "runs")
	t.Setenv("FLEETFORGE_SIMULATOR_SCENARIO_PATH", "scenario.json")
	t.Setenv("FLEETFORGE_SIMULATOR_USERNAME", "simulator")
	t.Setenv("FLEETFORGE_SIMULATOR_PASSWORD", "test-secret")
	t.Setenv("FLEETFORGE_SIMULATOR_ADMIN_ADDR", "127.0.0.1:8082")
	t.Setenv("FLEETFORGE_SIMULATOR_ADMIN_SECRET", "admin-secret")
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

func TestLoadObjectStoreUsesDedicatedBoundedConfiguration(t *testing.T) {
	t.Setenv("FLEETFORGE_ENV", "test")
	t.Setenv(objectStoreAddressKey, "127.0.0.1:8090")
	t.Setenv(objectStoreRootKey, t.TempDir())
	t.Setenv(objectStoreHMACSecretKey, base64.RawStdEncoding.EncodeToString([]byte("01234567890123456789012345678901")))
	loaded, err := LoadObjectStore()
	if err != nil {
		t.Fatalf("LoadObjectStore() error = %v", err)
	}
	if loaded.Address != "127.0.0.1:8090" || len(loaded.HMACKey) != 32 {
		t.Fatalf("LoadObjectStore() = %#v, want validated listener settings", loaded)
	}
}

func TestLoadArtifactWorkerDoesNotRequireUploadSigningKey(t *testing.T) {
	t.Setenv("FLEETFORGE_ENV", "test")
	t.Setenv(objectStoreRootKey, t.TempDir())
	t.Setenv(objectStoreHMACSecretKey, "")
	loaded, err := LoadArtifactWorker()
	if err != nil || loaded.Root == "" || loaded.Owner == "" {
		t.Fatalf("LoadArtifactWorker()=%#v err=%v", loaded, err)
	}
}

func TestLoadObjectStoreRedactsInvalidSecret(t *testing.T) {
	const secret = "credential-that-must-not-appear"
	t.Setenv(objectStoreAddressKey, "127.0.0.1:8090")
	t.Setenv(objectStoreRootKey, t.TempDir())
	t.Setenv(objectStoreHMACSecretKey, secret)
	_, err := LoadObjectStore()
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("LoadObjectStore() error = %v, want redacted failure", err)
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

func TestLoadPostgreSQLAcceptsNeonStandardConnectionVariables(t *testing.T) {
	t.Setenv(postgresApplicationDSNKey, "")
	t.Setenv(postgresMigrationDSNKey, "")
	t.Setenv(postgresApplicationDSNFallbackKey, "postgres://app:secret@pooled.example:5432/fleetforge?sslmode=require")
	t.Setenv(postgresMigrationDSNFallbackKey, "postgres://migrate:secret@direct.example:5432/fleetforge?sslmode=require")

	loaded, err := LoadPostgreSQL()
	if err != nil {
		t.Fatalf("LoadPostgreSQL() error = %v", err)
	}
	if loaded.ApplicationDSN == loaded.MigrationDSN {
		t.Fatal("LoadPostgreSQL() reused the pooled application DSN for migrations")
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
