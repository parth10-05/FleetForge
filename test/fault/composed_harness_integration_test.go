//go:build postgresintegration

package fault

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	testpostgres "github.com/parth10-05/fleetforge/test/support/postgres"
)

// TestComposedHarnessStartsRealSixTargetSimulator establishes the process and
// persistence boundary used by the P05 fault matrix. Worker and fault cases
// build on this runner; this test intentionally makes no update-success claim.
func TestComposedHarnessStartsRealSixTargetSimulator(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	harness := testpostgres.New(t)
	settings := config.PostgreSQL{ApplicationDSN: harness.ScopedDSN(t), MigrationDSN: harness.ScopedDSN(t), MaxConns: 2, AcquireTimeout: 15 * time.Second, TransactionTimeout: 10 * time.Second, StatementTimeout: 5 * time.Second, MaxConnLifetime: time.Minute, MaxConnIdleTime: time.Minute}
	root := repositoryRoot(t)
	if err := persistence.RunMigrations(ctx, settings, os.DirFS(filepath.Join(root, "db", "migrations"))); err != nil {
		t.Fatalf("migrate composed harness schema: %v", err)
	}

	runRoot := t.TempDir()
	address := freeAddress(t)
	adminAddress := freeAddress(t)
	adminSecret := "fault-harness-admin-secret"
	command := exec.CommandContext(ctx, "go", "run", "./cmd/redfish-simulator")
	command.Dir = root
	command.Env = append(os.Environ(),
		"FLEETFORGE_ENV=test",
		"FLEETFORGE_SIMULATOR_ADDR="+address,
		"FLEETFORGE_SIMULATOR_ADMIN_ADDR="+adminAddress,
		"FLEETFORGE_SIMULATOR_RUN_ROOT="+runRoot,
		"FLEETFORGE_SIMULATOR_SCENARIO_PATH="+filepath.Join(root, "test", "fixtures", "simulator", "v1", "healthy.json"),
		"FLEETFORGE_SIMULATOR_USERNAME=fault-harness",
		"FLEETFORGE_SIMULATOR_PASSWORD=fault-harness-password",
		"FLEETFORGE_SIMULATOR_ADMIN_SECRET="+adminSecret,
	)
	var output limitedBuffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatalf("start real simulator: %v", err)
	}
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Signal(os.Interrupt)
		}
		_ = command.Wait()
	})
	waitForStatus(t, ctx, "http://"+address+"/health/ready", http.Header{}, http.StatusOK)
	headers := http.Header{"X-FleetForge-Simulator-Admin": []string{adminSecret}}
	journal := waitForJSON(t, ctx, "http://"+adminAddress+"/admin/v1/journal", headers)
	if entries, ok := journal["entries"].(float64); !ok || entries < 1 {
		t.Fatalf("retained journal entries=%v, want startup evidence; simulator output=%s", journal["entries"], output.String())
	}
	status := waitForJSON(t, ctx, "http://"+adminAddress+"/admin/v1/status", headers)
	if ready, ok := status["ready"].(bool); !ok || !ready {
		t.Fatalf("simulator status=%v, want ready; simulator output=%s", status, output.String())
	}
	writeEvidence(t, harness.Schema, filepath.Join(root, "test", "fixtures", "simulator", "v1", "healthy.json"), status, journal)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().String()
}
func waitForStatus(t *testing.T, ctx context.Context, url string, headers http.Header, want int) {
	t.Helper()
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		request.Header = headers
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == want {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for %s: %v", url, ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func waitForJSON(t *testing.T, ctx context.Context, url string, headers http.Header) map[string]any {
	t.Helper()
	waitForStatus(t, ctx, url, headers, http.StatusOK)
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	request.Header = headers
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(value []byte) (int, error) {
	const max = 8192
	if b.Len() < max {
		_, _ = b.Buffer.Write(value[:min(len(value), max-b.Len())])
	}
	return len(value), nil
}
func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

// writeEvidence emits only redacted, machine-readable facts that can be
// retained from hosted execution. It intentionally omits DSNs, secrets,
// addresses, paths, headers, target credentials, and raw journal records.
func writeEvidence(t *testing.T, schema, scenarioPath string, status, journal map[string]any) {
	t.Helper()
	directory := os.Getenv("FLEETFORGE_FAULT_EVIDENCE_DIR")
	if directory == "" {
		return
	}
	rawScenario, err := os.ReadFile(scenarioPath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(rawScenario)
	report := map[string]any{
		"schema_version":    "fleetforge.p05-fault-evidence/v1",
		"case_id":           "P05-HARNESS-START",
		"schema":            schema,
		"scenario_sha256":   fmt.Sprintf("sha256:%x", digest),
		"simulator_run_id":  status["run_id"],
		"journal_entries":   journal["entries"],
		"journal_last_hash": journal["last_hash"],
		"result":            "PASS",
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(directory, "p05-harness-start.json.tmp")
	final := filepath.Join(directory, "p05-harness-start.json")
	if err := os.WriteFile(temporary, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, final); err != nil {
		t.Fatal(err)
	}
}
