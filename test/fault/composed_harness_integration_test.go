//go:build postgresintegration

package fault

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/execution"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	"github.com/parth10-05/fleetforge/internal/simulator/journal"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
	"github.com/parth10-05/fleetforge/test/support/executionfixture"
	testpostgres "github.com/parth10-05/fleetforge/test/support/postgres"
)

// TestComposedHarnessStartsRealSixTargetSimulator establishes the process and
// persistence boundary used by the P05 fault matrix. Worker and fault cases
// build on this runner; this test intentionally makes no update-success claim.
func TestComposedHarnessStartsRealSixTargetSimulator(t *testing.T) {
	setupCtx, cancelSetup := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelSetup()
	harness := testpostgres.New(t)
	settings := config.PostgreSQL{ApplicationDSN: harness.ScopedDSN(t), MigrationDSN: harness.ScopedDSN(t), MaxConns: 2, AcquireTimeout: 15 * time.Second, TransactionTimeout: 10 * time.Second, StatementTimeout: 5 * time.Second, MaxConnLifetime: time.Minute, MaxConnIdleTime: time.Minute}
	root := repositoryRoot(t)
	if err := persistence.RunMigrations(setupCtx, settings, os.DirFS(filepath.Join(root, "db", "migrations"))); err != nil {
		t.Fatalf("migrate composed harness schema: %v", err)
	}

	runRoot := t.TempDir()
	address := freeAddress(t)
	adminAddress := freeAddress(t)
	adminSecret := "fault-harness-admin-secret"
	simulatorPath := filepath.Join(runRoot, "redfish-simulator")
	if runtime.GOOS == "windows" {
		simulatorPath += ".exe"
	}
	build := exec.Command("go", "build", "-o", simulatorPath, "./cmd/redfish-simulator")
	build.Dir = root
	buildOutput, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build real simulator: %v: %s", err, buildOutput)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer func() { cancel() }()
	processEnv := append(os.Environ(),
		"FLEETFORGE_ENV=test",
		"FLEETFORGE_SIMULATOR_ADDR="+address,
		"FLEETFORGE_SIMULATOR_ADMIN_ADDR="+adminAddress,
		"FLEETFORGE_SIMULATOR_RUN_ROOT="+runRoot,
		"FLEETFORGE_SIMULATOR_SCENARIO_PATH="+filepath.Join(root, "test", "fixtures", "simulator", "v1", "accepted-harness.json"),
		"FLEETFORGE_SIMULATOR_USERNAME=fault-harness",
		"FLEETFORGE_SIMULATOR_PASSWORD=fault-harness-password",
		"FLEETFORGE_SIMULATOR_ADMIN_SECRET="+adminSecret,
	)
	var output limitedBuffer
	start := func() *exec.Cmd {
		command := exec.CommandContext(ctx, simulatorPath)
		command.Env = processEnv
		command.Stdout, command.Stderr = &output, &output
		if err := command.Start(); err != nil {
			t.Fatalf("start real simulator: %v", err)
		}
		return command
	}
	command := start()
	t.Cleanup(func() {
		stopSimulator(command)
	})
	waitForStatus(t, ctx, "http://"+address+"/health/ready", http.Header{}, http.StatusOK)
	headers := http.Header{"X-FleetForge-Simulator-Admin": []string{adminSecret}}
	journal := waitForJSON(t, ctx, "http://"+adminAddress+"/admin/v1/journal", headers)
	if _, ok := journal["entries"].(float64); !ok {
		t.Fatalf("retained journal entries=%v, want numeric journal status; simulator output=%s", journal["entries"], output.String())
	}
	status := waitForJSON(t, ctx, "http://"+adminAddress+"/admin/v1/status", headers)
	if ready, ok := status["ready"].(bool); !ok || !ready {
		t.Fatalf("simulator status=%v, want ready; simulator output=%s", status, output.String())
	}
	scenarioPath := filepath.Join(root, "test", "fixtures", "simulator", "v1", "accepted-harness.json")
	submitAcceptedUpdate(t, ctx, address, scenarioPath)
	journal = waitForJSON(t, ctx, "http://"+adminAddress+"/admin/v1/journal", headers)
	if entries, ok := journal["entries"].(float64); !ok || entries < 2 {
		t.Fatalf("retained journal entries=%v, want request and acceptance evidence; simulator output=%s", journal["entries"], output.String())
	}
	assertRetainedAcceptance(t, runRoot)
	assertFirmwareVersion(t, ctx, address, scenarioPath, "1.1")
	checkpoint := journal
	stopSimulator(command)
	cancel()
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	command = start()
	waitForStatus(t, ctx, "http://"+address+"/health/ready", http.Header{}, http.StatusOK)
	restarted := waitForJSON(t, ctx, "http://"+adminAddress+"/admin/v1/status", headers)
	if restarted["run_id"] != status["run_id"] {
		t.Fatalf("restored simulator run_id=%v, want %v", restarted["run_id"], status["run_id"])
	}
	restoredJournal := waitForJSON(t, ctx, "http://"+adminAddress+"/admin/v1/journal", headers)
	if restoredJournal["last_hash"] != checkpoint["last_hash"] || restoredJournal["entries"] != checkpoint["entries"] {
		t.Fatalf("restored journal=%v, want retained checkpoint=%v", restoredJournal, checkpoint)
	}
	assertFirmwareVersion(t, ctx, address, scenarioPath, "1.1")
	assertFalseProvenAbsentControlFails(t, runRoot)
	submitAcceptedUpdate(t, ctx, address, scenarioPath)
	assertDuplicateRequestControlFails(t, runRoot)
	journal = waitForJSON(t, ctx, "http://"+adminAddress+"/admin/v1/journal", headers)
	writeEvidence(t, harness.Schema, scenarioPath, status, journal)
}

// TestComposedHarnessPrewriteLossCreatesFencedChildAttempt joins a real
// simulator RF-F06 transport loss with the durable retry path. The simulator
// journal must show that no request reached the target before the only
// proof-backed child admission is allowed.
func TestComposedHarnessPrewriteLossCreatesFencedChildAttempt(t *testing.T) {
	setupCtx, cancelSetup := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelSetup()
	harness := testpostgres.New(t)
	settings := config.PostgreSQL{ApplicationDSN: harness.ScopedDSN(t), MigrationDSN: harness.ScopedDSN(t), MaxConns: 2, AcquireTimeout: 15 * time.Second, TransactionTimeout: 10 * time.Second, StatementTimeout: 5 * time.Second, MaxConnLifetime: time.Minute, MaxConnIdleTime: time.Minute}
	root := repositoryRoot(t)
	if err := persistence.RunMigrations(setupCtx, settings, os.DirFS(filepath.Join(root, "db", "migrations"))); err != nil {
		t.Fatalf("migrate composed prewrite schema: %v", err)
	}
	pool := harness.OpenPool(t)
	fixture, err := executionfixture.Seed(setupCtx, pool)
	if err != nil {
		t.Fatalf("seed durable fixture: %v", err)
	}
	if err := executionfixture.PrepareAdmission(setupCtx, pool, fixture, 7); err != nil {
		t.Fatalf("prepare durable fixture: %v", err)
	}
	if err := executionfixture.SeedPolicySnapshot(setupCtx, pool, fixture); err != nil {
		t.Fatalf("seed durable policy snapshot: %v", err)
	}

	runRoot := t.TempDir()
	scenarioPath := writePrewriteFailureScenario(t, runRoot, filepath.Join(root, "test", "fixtures", "simulator", "v1", "accepted-harness.json"))
	address, adminAddress := freeAddress(t), freeAddress(t)
	adminSecret := "fault-harness-prewrite-secret"
	simulatorPath := filepath.Join(runRoot, "redfish-simulator")
	if runtime.GOOS == "windows" {
		simulatorPath += ".exe"
	}
	build := exec.Command("go", "build", "-o", simulatorPath, "./cmd/redfish-simulator")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build prewrite simulator: %v: %s", err, output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var output limitedBuffer
	command := exec.CommandContext(ctx, simulatorPath)
	command.Env = append(os.Environ(),
		"FLEETFORGE_ENV=test",
		"FLEETFORGE_SIMULATOR_ADDR="+address,
		"FLEETFORGE_SIMULATOR_ADMIN_ADDR="+adminAddress,
		"FLEETFORGE_SIMULATOR_RUN_ROOT="+runRoot,
		"FLEETFORGE_SIMULATOR_SCENARIO_PATH="+scenarioPath,
		"FLEETFORGE_SIMULATOR_USERNAME=fault-harness",
		"FLEETFORGE_SIMULATOR_PASSWORD=fault-harness-password",
		"FLEETFORGE_SIMULATOR_ADMIN_SECRET="+adminSecret,
	)
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatalf("start prewrite simulator: %v", err)
	}
	t.Cleanup(func() { stopSimulator(command) })
	waitForStatus(t, ctx, "http://"+address+"/health/ready", http.Header{}, http.StatusOK)

	initial := executionfixture.AdmissionRequest(fixture, "5201")
	if err := persistence.AdmitOperation(ctx, pool, initial); err != nil {
		t.Fatalf("admit initial attempt: %v", err)
	}
	adapter := persistence.ExecutionAdapter{Pool: pool, Now: func() time.Time { return time.Now().UTC() }}
	submit, claimed, err := adapter.ClaimSubmitWork(ctx, "prewrite-harness", "00000000-0000-4000-8000-000000000201")
	if err != nil || !claimed {
		t.Fatalf("claim initial submit work=%+v claimed=%t err=%v", submit, claimed, err)
	}
	submitExpectedTransportLoss(t, ctx, address, scenarioPath, initial.FirmwareAttempt.AttemptID)
	headers := http.Header{"X-FleetForge-Simulator-Admin": []string{adminSecret}}
	journalStatus := waitForJSON(t, ctx, "http://"+adminAddress+"/admin/v1/journal", headers)
	if entries, ok := journalStatus["entries"].(float64); !ok || entries != 0 {
		t.Fatalf("prewrite journal entries=%v, want no target-side request evidence; simulator output=%s", journalStatus["entries"], output.String())
	}
	if err := adapter.CommitSubmitResult(ctx, submit, execution.SubmitResult{Class: execution.SubmitProvenPrewriteFailure, FinishedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("persist proven prewrite result: %v", err)
	}
	retry, claimed, err := adapter.ClaimPrecheckWork(ctx, "prewrite-harness", "00000000-0000-4000-8000-000000000202")
	if err != nil || !claimed {
		t.Fatalf("claim proof-backed retry work=%+v claimed=%t err=%v", retry, claimed, err)
	}
	child := executionfixture.AdmissionRequest(fixture, "5202")
	child.AdmissionID = initial.AdmissionID
	child.CommandID = retry.CommandID
	child.ExpectedTargetRevision = retry.ExpectedRevision
	child.CapturedCommandEpoch = 7
	child.ExpectedTargetState, child.NextTargetState = "RETRY_WAIT", "SUBMITTING"
	child.RetryParentAttemptID = initial.FirmwareAttempt.AttemptID
	child.ClaimedWorkID, child.ClaimedLeaseToken, child.ClaimedDeliveryGeneration = retry.DueWorkID, retry.LeaseToken, retry.DeliveryGeneration
	child.DueWorkID = "00000000-0000-0000-5202-000000000099"
	child.FirmwareAttempt.AttemptID = "00000000-0000-0000-5202-000000000010"
	child.StepAttemptID = "00000000-0000-0000-5202-000000000011"
	if err := persistence.AdmitRetryOperation(ctx, pool, child); err != nil {
		t.Fatalf("admit proof-backed child: %v", err)
	}
	assertExactlyOneChildAdmission(t, ctx, pool, initial.AdmissionID, initial.FirmwareAttempt.AttemptID, child.FirmwareAttempt.AttemptID, retry)
}

func stopSimulator(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = command.Process.Signal(os.Interrupt)
	_ = command.Wait()
}

func submitAcceptedUpdate(t *testing.T, ctx context.Context, address, scenarioPath string) {
	t.Helper()
	input := readScenario(t, scenarioPath)
	assertAcceptanceFleet(t, input)
	targetID, err := scenario.ID(input, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("firmware-harness-artifact")
	digest := sha256.Sum256(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+address+"/virtual-bmcs/"+targetID+"/redfish/v1/UpdateService/update", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth("fault-harness", "fault-harness-password")
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-FleetForge-Expected-Version", "1.1")
	request.Header.Set("X-FleetForge-Artifact-SHA256", hex.EncodeToString(digest[:]))
	request.Header.Set("X-FleetForge-Attempt", "harness-attempt-1")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("update status=%d, want %d: %s", response.StatusCode, http.StatusAccepted, responseBody)
	}
}

func submitExpectedTransportLoss(t *testing.T, ctx context.Context, address, scenarioPath, attemptID string) {
	t.Helper()
	input := readScenario(t, scenarioPath)
	targetID, err := scenario.ID(input, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("firmware-prewrite-artifact")
	digest := sha256.Sum256(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+address+"/virtual-bmcs/"+targetID+"/redfish/v1/UpdateService/update", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth("fault-harness", "fault-harness-password")
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-FleetForge-Expected-Version", "1.1")
	request.Header.Set("X-FleetForge-Artifact-SHA256", hex.EncodeToString(digest[:]))
	request.Header.Set("X-FleetForge-Attempt", attemptID)
	response, err := http.DefaultClient.Do(request)
	if err == nil {
		response.Body.Close()
		t.Fatal("RF-F06 update unexpectedly returned a response")
	}
}

func writePrewriteFailureScenario(t *testing.T, directory, source string) string {
	t.Helper()
	input := readScenario(t, source)
	input.ScenarioID = "p05-prewrite-failure"
	input.Seed = "5201"
	input.Faults = append(input.Faults, scenario.Fault{FaultID: "prewrite-loss", Name: "RF-F06", Scope: "all", Trigger: scenario.Trigger{Kind: "logical_time", Value: 0}, Consumption: 1, Priority: 1})
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "prewrite-failure.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertExactlyOneChildAdmission(t *testing.T, ctx context.Context, pool *pgxpool.Pool, admissionID, parentAttemptID, childAttemptID string, retry execution.WorkFence) {
	t.Helper()
	var parent, active, state, delivery, submitCommandID string
	var children, reservations, submits int
	if err := pool.QueryRow(ctx, `SELECT a.parent_attempt_id::text,s.active_attempt_id::text,tu.state
FROM firmware_update_attempts a JOIN submission_admissions s ON s.id=a.submission_admission_id
JOIN target_updates tu ON tu.id=a.target_update_id WHERE a.id=$1::uuid`, childAttemptID).Scan(&parent, &active, &state); err != nil {
		t.Fatal(err)
	}
	if parent != parentAttemptID || active != childAttemptID || state != "SUBMITTING" {
		t.Fatalf("child parent=%s active=%s state=%s", parent, active, state)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM firmware_update_attempts WHERE submission_admission_id=$1::uuid AND parent_attempt_id IS NOT NULL", admissionID).Scan(&children); err != nil || children != 1 {
		t.Fatalf("children=%d err=%v, want one", children, err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM operation_reservations WHERE admission_id=$1::uuid", admissionID).Scan(&reservations); err != nil || reservations != 3 {
		t.Fatalf("reservations=%d err=%v, want three retained", reservations, err)
	}
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM due_work WHERE id=$1::uuid", retry.DueWorkID).Scan(&delivery); err != nil || delivery != "COMPLETED" {
		t.Fatalf("retry delivery=%s err=%v, want completed", delivery, err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM due_work WHERE work_type='SUBMIT' AND delivery_state='PENDING'").Scan(&submits); err != nil || submits != 1 {
		t.Fatalf("pending submits=%d err=%v, want one child", submits, err)
	}
	if err := pool.QueryRow(ctx, "SELECT command_id::text FROM due_work WHERE work_type='SUBMIT' AND delivery_state='PENDING'").Scan(&submitCommandID); err != nil || submitCommandID == retry.CommandID {
		t.Fatalf("child submit command=%s err=%v, want distinct retry identity", submitCommandID, err)
	}
}

func readScenario(t *testing.T, scenarioPath string) scenario.Scenario {
	t.Helper()
	rawScenario, err := os.ReadFile(scenarioPath)
	if err != nil {
		t.Fatal(err)
	}
	input, err := scenario.Decode(rawScenario)
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func assertFirmwareVersion(t *testing.T, ctx context.Context, address, scenarioPath, want string) {
	t.Helper()
	input := readScenario(t, scenarioPath)
	targetID, err := scenario.ID(input, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + address + "/virtual-bmcs/" + targetID + "/redfish/v1/UpdateService/FirmwareInventory/BMC"
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.SetBasicAuth("fault-harness", "fault-harness-password")
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			var body map[string]any
			decodeErr := json.NewDecoder(response.Body).Decode(&body)
			response.Body.Close()
			if decodeErr == nil && body["Version"] == want {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for restored firmware version %q: %v", want, ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func assertAcceptanceFleet(t *testing.T, input scenario.Scenario) {
	t.Helper()
	if len(input.Targets) < 6 || len(input.Targets) > 10 {
		t.Fatalf("acceptance scenario targets=%d, want 6 through 10", len(input.Targets))
	}
	racks := make(map[string]struct{}, len(input.Targets))
	for _, target := range input.Targets {
		racks[target.Rack] = struct{}{}
	}
	if len(racks) < 2 {
		t.Fatalf("acceptance scenario racks=%d, want at least two", len(racks))
	}
}

func assertRetainedAcceptance(t *testing.T, runRoot string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(runRoot, "*", "journal.jsonl"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("retained journal paths=%v err=%v", paths, err)
	}
	entries, err := journal.Load(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	received, accepted := -1, -1
	for index, entry := range entries {
		if entry.Payload.CorrelationID != "harness-attempt-1" {
			continue
		}
		if entry.Kind == journal.RequestReceived {
			received = index
		}
		if entry.Kind == journal.UpdateAccepted {
			accepted = index
		}
	}
	if received < 0 || accepted < 0 || received >= accepted {
		t.Fatalf("retained acceptance ordering received=%d accepted=%d", received, accepted)
	}
}

func assertDuplicateRequestControlFails(t *testing.T, runRoot string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(runRoot, "*", "journal.jsonl"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("retained journal paths=%v err=%v", paths, err)
	}
	entries, err := journal.Load(paths[0])
	if err != nil || len(entries) == 0 {
		t.Fatalf("load retained duplicate-control journal entries=%d err=%v", len(entries), err)
	}
	acceptance := execution.AcceptanceJournal{RunID: entries[0].RunID, CheckpointHash: "sha256:" + entries[len(entries)-1].Hash}
	requestFingerprint := ""
	for _, entry := range entries {
		if entry.Payload.CorrelationID != "harness-attempt-1" {
			continue
		}
		if requestFingerprint == "" {
			requestFingerprint = "sha256:" + entry.Payload.Fingerprint
		}
		if entry.Kind == journal.RequestReceived || entry.Kind == journal.UpdateAccepted {
			acceptance.Entries = append(acceptance.Entries, execution.AcceptanceEntry{
				AttemptID:          entry.Payload.CorrelationID,
				RequestFingerprint: "sha256:" + entry.Payload.Fingerprint,
				Received:           entry.Kind == journal.RequestReceived,
				Accepted:           entry.Kind == journal.UpdateAccepted,
				Sequence:           int64(entry.Sequence),
			})
		}
	}
	if requestFingerprint == "" {
		t.Fatal("duplicate-control journal has no request fingerprint")
	}
	err = execution.VerifyIndependentJournal([]execution.OracleAttempt{{
		AttemptID:          "harness-attempt-1",
		IntendedUpdateID:   "harness-target-update-1",
		RequestFingerprint: requestFingerprint,
		Resolution:         "ACCEPTED",
	}}, acceptance)
	if err == nil {
		t.Fatal("duplicate request negative control unexpectedly passed the independent journal oracle")
	}
}

func assertFalseProvenAbsentControlFails(t *testing.T, runRoot string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(runRoot, "*", "journal.jsonl"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("retained journal paths=%v err=%v", paths, err)
	}
	entries, err := journal.Load(paths[0])
	if err != nil || len(entries) == 0 {
		t.Fatalf("load retained false-absence-control journal entries=%d err=%v", len(entries), err)
	}
	acceptance := execution.AcceptanceJournal{RunID: entries[0].RunID, CheckpointHash: "sha256:" + entries[len(entries)-1].Hash}
	requestFingerprint := ""
	for _, entry := range entries {
		if entry.Payload.CorrelationID != "harness-attempt-1" {
			continue
		}
		if requestFingerprint == "" {
			requestFingerprint = "sha256:" + entry.Payload.Fingerprint
		}
		if entry.Kind == journal.RequestReceived || entry.Kind == journal.UpdateAccepted {
			acceptance.Entries = append(acceptance.Entries, execution.AcceptanceEntry{
				AttemptID:          entry.Payload.CorrelationID,
				RequestFingerprint: "sha256:" + entry.Payload.Fingerprint,
				Received:           entry.Kind == journal.RequestReceived,
				Accepted:           entry.Kind == journal.UpdateAccepted,
				Sequence:           int64(entry.Sequence),
			})
		}
	}
	if requestFingerprint == "" {
		t.Fatal("false-absence-control journal has no request fingerprint")
	}
	err = execution.VerifyIndependentJournal([]execution.OracleAttempt{{
		AttemptID:          "harness-attempt-1",
		IntendedUpdateID:   "harness-target-update-1",
		RequestFingerprint: requestFingerprint,
		Resolution:         "PROVEN_ABSENT",
		LateCallerExcluded: true,
	}}, acceptance)
	if err == nil {
		t.Fatal("false proven-absent negative control unexpectedly passed the independent journal oracle")
	}
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
		"schema_version":                      "fleetforge.p05-fault-evidence/v1",
		"case_id":                             "P05-HARNESS-ACCEPTANCE-NEGATIVE-CONTROL",
		"schema":                              schema,
		"scenario_sha256":                     fmt.Sprintf("sha256:%x", digest),
		"simulator_run_id":                    status["run_id"],
		"journal_entries":                     journal["entries"],
		"journal_last_hash":                   journal["last_hash"],
		"duplicate_request_oracle_rejected":   true,
		"false_proven_absent_oracle_rejected": true,
		"result":                              "PASS",
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
