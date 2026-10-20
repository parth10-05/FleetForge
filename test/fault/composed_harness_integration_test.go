//go:build postgresintegration

package fault

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/eventhint"
	"github.com/parth10-05/fleetforge/internal/application/execution"
	"github.com/parth10-05/fleetforge/internal/delivery/eventhinthttp"
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

// TestComposedHarnessPostAcceptLossAdoptsRetainedAcceptance proves that the
// RF-F07 response-loss boundary is resolved from the simulator's independent
// journal. The reconciler must schedule observation for the original attempt;
// it must not issue a second submission.
func TestComposedHarnessPostAcceptLossAdoptsRetainedAcceptance(t *testing.T) {
	setupCtx, cancelSetup := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelSetup()
	harness := testpostgres.New(t)
	settings := config.PostgreSQL{ApplicationDSN: harness.ScopedDSN(t), MigrationDSN: harness.ScopedDSN(t), MaxConns: 2, AcquireTimeout: 15 * time.Second, TransactionTimeout: 10 * time.Second, StatementTimeout: 5 * time.Second, MaxConnLifetime: time.Minute, MaxConnIdleTime: time.Minute}
	root := repositoryRoot(t)
	if err := persistence.RunMigrations(setupCtx, settings, os.DirFS(filepath.Join(root, "db", "migrations"))); err != nil {
		t.Fatalf("migrate composed post-accept schema: %v", err)
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
	address, adminAddress := freeAddress(t), freeAddress(t)
	adminSecret := "fault-harness-post-accept-secret"
	simulatorPath := filepath.Join(runRoot, "redfish-simulator")
	if runtime.GOOS == "windows" {
		simulatorPath += ".exe"
	}
	build := exec.Command("go", "build", "-o", simulatorPath, "./cmd/redfish-simulator")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build post-accept simulator: %v: %s", err, output)
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
		"FLEETFORGE_SIMULATOR_SCENARIO_PATH="+filepath.Join(root, "test", "fixtures", "simulator", "v1", "post-accept-disconnect.json"),
		"FLEETFORGE_SIMULATOR_USERNAME=fault-harness",
		"FLEETFORGE_SIMULATOR_PASSWORD=fault-harness-password",
		"FLEETFORGE_SIMULATOR_ADMIN_SECRET="+adminSecret,
	)
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatalf("start post-accept simulator: %v", err)
	}
	t.Cleanup(func() { stopSimulator(command) })
	waitForStatus(t, ctx, "http://"+address+"/health/ready", http.Header{}, http.StatusOK)

	body := []byte("firmware-postaccept-artifact")
	digest := sha256.Sum256(body)
	initial := executionfixture.AdmissionRequest(fixture, "5205")
	initial.FirmwareAttempt.RequestFingerprint = "sha256:" + hex.EncodeToString(digest[:])
	initial.FirmwareAttempt.ExpectedVersion = "1.1"
	if err := persistence.AdmitOperation(ctx, pool, initial); err != nil {
		t.Fatalf("admit initial attempt: %v", err)
	}
	adapter := persistence.ExecutionAdapter{Pool: pool, Now: func() time.Time { return time.Now().UTC() }}
	submit, claimed, err := adapter.ClaimSubmitWork(ctx, "post-accept-harness", "00000000-0000-4000-8000-000000000205")
	if err != nil || !claimed {
		t.Fatalf("claim initial submit work=%+v claimed=%t err=%v", submit, claimed, err)
	}
	submitExpectedPostAcceptTransportLoss(t, ctx, address, filepath.Join(root, "test", "fixtures", "simulator", "v1", "post-accept-disconnect.json"), initial.FirmwareAttempt.AttemptID, body)
	acceptance := retainedAcceptanceForAttempt(t, runRoot, initial.FirmwareAttempt.AttemptID)
	if len(acceptance.Entries) != 2 || !acceptance.Entries[0].Received || !acceptance.Entries[1].Accepted {
		t.Fatalf("RF-F07 journal entries=%+v, want exactly ordered receive and acceptance evidence", acceptance.Entries)
	}
	if err := adapter.CommitSubmitResult(ctx, submit, execution.SubmitResult{Class: execution.SubmitPossiblyAccepted, ResponseFingerprint: harnessDigest('f'), FinishedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("persist response-loss ambiguity: %v", err)
	}
	worker := execution.ReconciliationWorker{Claimer: adapter, Repository: adapter, Journal: retainedJournalReader{journal: acceptance}, RunID: acceptance.RunID, Now: func() time.Time { return time.Now().UTC() }}
	processed, err := worker.RunOnce(ctx, "post-accept-harness", "00000000-0000-4000-8000-000000000206")
	if err != nil || !processed {
		t.Fatalf("reconcile retained acceptance processed=%t err=%v", processed, err)
	}
	if err := execution.VerifyPersistedJournal(ctx, adapter, acceptance); err != nil {
		t.Fatalf("verify adopted durable attempt against retained journal: %v", err)
	}
	var attemptStatus, updateState string
	var pendingSubmits, pendingObservations int
	if err := pool.QueryRow(ctx, `SELECT a.status,tu.state FROM firmware_update_attempts a JOIN target_updates tu ON tu.id=a.target_update_id WHERE a.id=$1::uuid`, initial.FirmwareAttempt.AttemptID).Scan(&attemptStatus, &updateState); err != nil {
		t.Fatal(err)
	}
	if attemptStatus != "ADOPTED" || updateState != "UPDATE_ACCEPTED" {
		t.Fatalf("adopted lifecycle attempt=%s update=%s", attemptStatus, updateState)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM due_work WHERE work_type='SUBMIT' AND delivery_state='PENDING'").Scan(&pendingSubmits); err != nil || pendingSubmits != 0 {
		t.Fatalf("pending submits=%d err=%v, want no second target request", pendingSubmits, err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM due_work WHERE work_type='TASK_OBSERVE' AND delivery_state='PENDING'").Scan(&pendingObservations); err != nil || pendingObservations != 1 {
		t.Fatalf("pending task observations=%d err=%v, want one adopted observation", pendingObservations, err)
	}
}

// TestComposedHarnessDuplicateCallbackIsDeduplicated drives a real simulator
// EventService subscription through the signed HTTP ingress and confirms that
// RF-F01's duplicate delivery remains only an advisory, durable single receipt.
func TestComposedHarnessDuplicateCallbackIsDeduplicated(t *testing.T) {
	setupCtx, cancelSetup := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelSetup()
	harness := testpostgres.New(t)
	settings := config.PostgreSQL{ApplicationDSN: harness.ScopedDSN(t), MigrationDSN: harness.ScopedDSN(t), MaxConns: 2, AcquireTimeout: 15 * time.Second, TransactionTimeout: 10 * time.Second, StatementTimeout: 5 * time.Second, MaxConnLifetime: time.Minute, MaxConnIdleTime: time.Minute}
	root := repositoryRoot(t)
	if err := persistence.RunMigrations(setupCtx, settings, os.DirFS(filepath.Join(root, "db", "migrations"))); err != nil {
		t.Fatalf("migrate callback schema: %v", err)
	}
	pool := harness.OpenPool(t)
	fixture, err := executionfixture.Seed(setupCtx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := executionfixture.PrepareAdmission(setupCtx, pool, fixture, 7); err != nil {
		t.Fatal(err)
	}
	if err := executionfixture.SeedPolicySnapshot(setupCtx, pool, fixture); err != nil {
		t.Fatal(err)
	}
	callbackKey := bytes.Repeat([]byte("c"), 32)
	var callbackAttempts atomic.Int64
	var callbackStatus atomic.Int64
	callbackStore := &callbackStoreRecorder{store: persistence.EventHintStore{Pool: pool}}
	callbackRoutes := eventhinthttp.New(callbackKey, callbackStore, time.Now).Routes()
	callback := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		callbackAttempts.Add(1)
		callbackRoutes.ServeHTTP(&callbackStatusRecorder{ResponseWriter: writer, status: &callbackStatus}, request)
	}))
	defer callback.Close()
	runRoot, address, adminAddress := t.TempDir(), freeAddress(t), freeAddress(t)
	scenarioPath := filepath.Join(root, "test", "fixtures", "simulator", "v1", "accepted-harness.json")
	input := readScenario(t, scenarioPath)
	input.ScenarioID = "duplicate-callback-six-targets"
	// Give the durable accepted-result transaction time to commit before the
	// first advisory callback fires. The production invariant is that callbacks
	// may accelerate observation only after a durable attempt exists.
	input.Timing.UploadMS = 100
	input.Timing.TaskStepMS = 100
	input.Timing.RebootMS = 100
	input.Timing.InventoryVisibilityMS = 100
	input.Timing.EventMS = 100
	input.Faults = append(input.Faults, scenario.Fault{FaultID: "duplicate-callback", Name: "RF-F01", Scope: "all", Trigger: scenario.Trigger{Kind: "logical_time", Value: 0}, Consumption: 1, Priority: 1})
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	casePath := filepath.Join(runRoot, "duplicate-callback.json")
	if err := os.WriteFile(casePath, append(encoded, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	targetID, err := scenario.ID(input, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(setupCtx, "UPDATE targets SET target_key=$1 WHERE id=$2::uuid", targetID, fixture.TargetID); err != nil {
		t.Fatal(err)
	}
	simulatorPath := filepath.Join(runRoot, "redfish-simulator")
	if runtime.GOOS == "windows" {
		simulatorPath += ".exe"
	}
	build := exec.Command("go", "build", "-o", simulatorPath, "./cmd/redfish-simulator")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build callback simulator: %v: %s", err, output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, simulatorPath)
	command.Env = append(os.Environ(), "FLEETFORGE_ENV=test", "FLEETFORGE_READINESS_TIMEOUT=5s", "FLEETFORGE_SIMULATOR_ADDR="+address, "FLEETFORGE_SIMULATOR_ADMIN_ADDR="+adminAddress, "FLEETFORGE_SIMULATOR_RUN_ROOT="+runRoot, "FLEETFORGE_SIMULATOR_SCENARIO_PATH="+casePath, "FLEETFORGE_SIMULATOR_USERNAME=fault-harness", "FLEETFORGE_SIMULATOR_PASSWORD=fault-harness-password", "FLEETFORGE_SIMULATOR_ADMIN_SECRET=callback-admin-secret", "FLEETFORGE_SIMULATOR_CALLBACK_HMAC_SECRET="+base64.RawStdEncoding.EncodeToString(callbackKey))
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopSimulator(command) })
	waitForStatus(t, ctx, "http://"+address+"/health/ready", http.Header{}, http.StatusOK)
	subscribeCallback(t, ctx, address, targetID, callback.URL+"/v1/events/synthetic")
	initial := executionfixture.AdmissionRequest(fixture, "5206")
	if err := persistence.AdmitOperation(ctx, pool, initial); err != nil {
		t.Fatal(err)
	}
	adapter := persistence.ExecutionAdapter{Pool: pool, Now: func() time.Time { return time.Now().UTC() }}
	work, claimed, err := adapter.ClaimSubmitWork(ctx, "callback-harness", "00000000-0000-4000-8000-000000000206")
	if err != nil || !claimed {
		t.Fatalf("claim submit=%+v claimed=%t err=%v", work, claimed, err)
	}
	remoteTask := submitAcceptedUpdateForAttempt(t, ctx, address, input, initial.FirmwareAttempt.AttemptID)
	if err := adapter.CommitSubmitResult(ctx, work, execution.SubmitResult{Class: execution.SubmitAccepted, RemoteTaskReference: remoteTask, ResponseFingerprint: harnessDigest('c'), FinishedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	for {
		var receipts int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM execution_event_receipts").Scan(&receipts); err == nil && receipts == 1 {
			break
		}
		select {
		case <-ctx.Done():
			var quarantined int
			_ = pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM execution_event_quarantine").Scan(&quarantined)
			t.Fatalf("timed out waiting for signed callback receipt: callback_attempts=%d last_status=%d quarantined=%d record_error=%q", callbackAttempts.Load(), callbackStatus.Load(), quarantined, callbackStore.LastError())
		case <-time.After(20 * time.Millisecond):
		}
	}
	var receipts, sequence, observations int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM execution_event_receipts").Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("receipts=%d err=%v, want one deduplicated receipt", receipts, err)
	}
	if err := pool.QueryRow(ctx, "SELECT sequence FROM execution_event_heads WHERE target_key=$1", targetID).Scan(&sequence); err != nil || sequence < 1 {
		t.Fatalf("sequence=%d err=%v", sequence, err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM due_work WHERE work_type='TASK_OBSERVE' AND delivery_state='PENDING'").Scan(&observations); err != nil || observations < 1 {
		t.Fatalf("task observations=%d err=%v", observations, err)
	}
}

type callbackStatusRecorder struct {
	http.ResponseWriter
	status *atomic.Int64
}

type callbackStoreRecorder struct {
	store   eventhinthttp.Store
	lastErr atomic.Value
}

func (store *callbackStoreRecorder) RecordEventHint(ctx context.Context, hint eventhint.Hint) (string, error) {
	disposition, err := store.store.RecordEventHint(ctx, hint)
	if err != nil {
		store.lastErr.Store(err.Error())
	}
	return disposition, err
}

func (store *callbackStoreRecorder) QuarantineEventHint(ctx context.Context, body []byte, reason string, receivedAt time.Time) error {
	return store.store.QuarantineEventHint(ctx, body, reason, receivedAt)
}

func (store *callbackStoreRecorder) LastError() string {
	if value := store.lastErr.Load(); value != nil {
		return value.(string)
	}
	return ""
}

func (writer *callbackStatusRecorder) WriteHeader(status int) {
	writer.status.Store(int64(status))
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *callbackStatusRecorder) Write(body []byte) (int, error) {
	if writer.status.Load() == 0 {
		writer.status.Store(http.StatusOK)
	}
	return writer.ResponseWriter.Write(body)
}

// TestComposedHarnessAdminProvenAbsentCreatesFencedChildAttempt proves that
// an administrator can authorize a replacement only after a retained real
// simulator run has no matching request evidence. The durable retry still
// carries the normal revision, lease, and reservation fences.
func TestComposedHarnessAdminProvenAbsentCreatesFencedChildAttempt(t *testing.T) {
	setupCtx, cancelSetup := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelSetup()
	harness := testpostgres.New(t)
	settings := config.PostgreSQL{ApplicationDSN: harness.ScopedDSN(t), MigrationDSN: harness.ScopedDSN(t), MaxConns: 2, AcquireTimeout: 15 * time.Second, TransactionTimeout: 10 * time.Second, StatementTimeout: 5 * time.Second, MaxConnLifetime: time.Minute, MaxConnIdleTime: time.Minute}
	root := repositoryRoot(t)
	if err := persistence.RunMigrations(setupCtx, settings, os.DirFS(filepath.Join(root, "db", "migrations"))); err != nil {
		t.Fatalf("migrate composed admin-absence schema: %v", err)
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
	address, adminAddress := freeAddress(t), freeAddress(t)
	adminSecret := "fault-harness-admin-absence-secret"
	simulatorPath := filepath.Join(runRoot, "redfish-simulator")
	if runtime.GOOS == "windows" {
		simulatorPath += ".exe"
	}
	build := exec.Command("go", "build", "-o", simulatorPath, "./cmd/redfish-simulator")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build admin-absence simulator: %v: %s", err, output)
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
		"FLEETFORGE_SIMULATOR_SCENARIO_PATH="+filepath.Join(root, "test", "fixtures", "simulator", "v1", "accepted-harness.json"),
		"FLEETFORGE_SIMULATOR_USERNAME=fault-harness",
		"FLEETFORGE_SIMULATOR_PASSWORD=fault-harness-password",
		"FLEETFORGE_SIMULATOR_ADMIN_SECRET="+adminSecret,
	)
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatalf("start admin-absence simulator: %v", err)
	}
	t.Cleanup(func() { stopSimulator(command) })
	waitForStatus(t, ctx, "http://"+address+"/health/ready", http.Header{}, http.StatusOK)
	headers := http.Header{"X-FleetForge-Simulator-Admin": []string{adminSecret}}
	journalStatus := waitForJSON(t, ctx, "http://"+adminAddress+"/admin/v1/journal", headers)
	if entries, ok := journalStatus["entries"].(float64); !ok || entries != 0 {
		t.Fatalf("admin absence journal entries=%v, want no target request evidence; simulator output=%s", journalStatus["entries"], output.String())
	}

	initial := executionfixture.AdmissionRequest(fixture, "5203")
	if err := persistence.AdmitOperation(ctx, pool, initial); err != nil {
		t.Fatalf("admit initial ambiguous attempt: %v", err)
	}
	adapter := persistence.ExecutionAdapter{Pool: pool, Now: func() time.Time { return time.Now().UTC() }}
	submit, claimed, err := adapter.ClaimSubmitWork(ctx, "admin-absence-harness", "00000000-0000-4000-8000-000000000203")
	if err != nil || !claimed {
		t.Fatalf("claim initial submit work=%+v claimed=%t err=%v", submit, claimed, err)
	}
	if err := adapter.CommitSubmitResult(ctx, submit, execution.SubmitResult{Class: execution.SubmitPossiblyAccepted, ResponseFingerprint: harnessDigest('d'), FinishedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("persist ambiguous result: %v", err)
	}
	now := time.Now().UTC()
	if _, err := adapter.ResolveAttention(ctx, execution.AdminResolutionCommand{
		Principal: execution.VerifiedPrincipal{Subject: "admin", IssuerClass: "local-test", Roles: []string{"firmware:resolve"}, Verified: true, AuthenticatedAt: now, ExpiresAt: now.Add(time.Minute)},
		Work:      execution.WorkFence{CommandID: "00000000-0000-0000-5203-000000000098", ExpectedRevision: 2}, AttemptID: initial.FirmwareAttempt.AttemptID,
		Action: "RETRY_PROVEN_ABSENT", Reason: "harness.proven_absent", EvidenceFingerprint: harnessDigest('e'), JournalConfirmedAbsent: true, LateCallerExcluded: true,
		LateCallerExclusionMethod: "retained-journal-empty", ProofReference: "simulator://retained-empty-journal", Now: now,
	}); err != nil {
		t.Fatalf("persist administrator proven-absent resolution: %v", err)
	}
	retry, claimed, err := adapter.ClaimPrecheckWork(ctx, "admin-absence-harness", "00000000-0000-4000-8000-000000000204")
	if err != nil || !claimed {
		t.Fatalf("claim administrator retry work=%+v claimed=%t err=%v", retry, claimed, err)
	}
	child := executionfixture.AdmissionRequest(fixture, "5204")
	child.AdmissionID, child.CommandID, child.ExpectedTargetRevision, child.CapturedCommandEpoch = initial.AdmissionID, retry.CommandID, retry.ExpectedRevision, 7
	child.ExpectedTargetState, child.NextTargetState, child.RetryParentAttemptID = "RETRY_WAIT", "SUBMITTING", initial.FirmwareAttempt.AttemptID
	child.ClaimedWorkID, child.ClaimedLeaseToken, child.ClaimedDeliveryGeneration = retry.DueWorkID, retry.LeaseToken, retry.DeliveryGeneration
	child.DueWorkID, child.FirmwareAttempt.AttemptID, child.StepAttemptID = "00000000-0000-0000-5204-000000000099", "00000000-0000-0000-5204-000000000010", "00000000-0000-0000-5204-000000000011"
	if err := persistence.AdmitRetryOperation(ctx, pool, child); err != nil {
		t.Fatalf("admit administrator-authorized child: %v", err)
	}
	assertExactlyOneChildAdmission(t, ctx, pool, initial.AdmissionID, initial.FirmwareAttempt.AttemptID, child.FirmwareAttempt.AttemptID, retry)
	childSubmit, claimed, err := adapter.ClaimSubmitWork(ctx, "admin-absence-harness", "00000000-0000-4000-8000-000000000205")
	if err != nil || !claimed {
		t.Fatalf("claim child submit work=%+v claimed=%t err=%v", childSubmit, claimed, err)
	}
	loaded, err := adapter.LoadAttempt(ctx, childSubmit)
	if err != nil || loaded.AttemptID != child.FirmwareAttempt.AttemptID || loaded.ParentAttemptID != initial.FirmwareAttempt.AttemptID {
		t.Fatalf("loaded child attempt=%+v err=%v", loaded, err)
	}
}

func stopSimulator(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = command.Process.Signal(os.Interrupt)
	_ = command.Wait()
}

func harnessDigest(character byte) string {
	return "sha256:" + string(bytes.Repeat([]byte{character}, 64))
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

func subscribeCallback(t *testing.T, ctx context.Context, address, targetID, destination string) {
	t.Helper()
	body := []byte(`{"Destination":"` + destination + `","Protocol":"Redfish","SubscriptionType":"RedfishEvent"}`)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+address+"/virtual-bmcs/"+targetID+"/redfish/v1/EventService/Subscriptions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth("fault-harness", "fault-harness-password")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("subscribe status=%d", response.StatusCode)
	}
}

func submitAcceptedUpdateForAttempt(t *testing.T, ctx context.Context, address string, input scenario.Scenario, attemptID string) string {
	t.Helper()
	targetID, err := scenario.ID(input, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("firmware-callback-artifact")
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
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("update status=%d", response.StatusCode)
	}
	if location := response.Header.Get("Location"); location != "" {
		prefix := "/virtual-bmcs/" + targetID
		if strings.HasPrefix(location, prefix) {
			return strings.TrimPrefix(location, prefix)
		}
		t.Fatalf("accepted update location=%q, want virtual BMC task URI", location)
	}
	t.Fatal("accepted update omitted task Location")
	return ""
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

func submitExpectedPostAcceptTransportLoss(t *testing.T, ctx context.Context, address, scenarioPath, attemptID string, body []byte) {
	t.Helper()
	input := readScenario(t, scenarioPath)
	targetID, err := scenario.ID(input, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal("RF-F07 update unexpectedly returned a response")
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

func retainedAcceptanceForAttempt(t *testing.T, runRoot, attemptID string) execution.AcceptanceJournal {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(runRoot, "*", "journal.jsonl"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("retained journal paths=%v err=%v", paths, err)
	}
	entries, err := journal.Load(paths[0])
	if err != nil || len(entries) == 0 {
		t.Fatalf("load retained RF-F07 journal entries=%d err=%v", len(entries), err)
	}
	acceptance := execution.AcceptanceJournal{RunID: entries[0].RunID, CheckpointHash: "sha256:" + entries[len(entries)-1].Hash}
	for _, entry := range entries {
		if entry.Payload.CorrelationID != attemptID || (entry.Kind != journal.RequestReceived && entry.Kind != journal.UpdateAccepted) {
			continue
		}
		acceptance.Entries = append(acceptance.Entries, execution.AcceptanceEntry{AttemptID: attemptID, RequestFingerprint: "sha256:" + entry.Payload.Fingerprint, Received: entry.Kind == journal.RequestReceived, Accepted: entry.Kind == journal.UpdateAccepted, Sequence: int64(entry.Sequence)})
	}
	if len(acceptance.Entries) == 0 {
		t.Fatalf("retained RF-F07 journal has no evidence for attempt %s", attemptID)
	}
	return acceptance
}

type retainedJournalReader struct{ journal execution.AcceptanceJournal }

func (reader retainedJournalReader) ReadAcceptance(_ context.Context, runID, _ string, _, _ time.Time) (execution.AcceptanceJournal, error) {
	if runID != reader.journal.RunID {
		return execution.AcceptanceJournal{}, fmt.Errorf("unexpected simulator run %q", runID)
	}
	return reader.journal, nil
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
