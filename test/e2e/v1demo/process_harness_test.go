//go:build postgresintegration

package v1demo

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/delivery/controlhttp"
	"github.com/parth10-05/fleetforge/internal/platform/localoidc"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

// composedDemo is the release-path composition used by P06 acceptance cases.
// It starts repository binaries against the test's isolated schema; API setup
// remains public HTTP and SQL is reserved for read-only test oracles.
type composedDemo struct {
	apiURL       string
	simulatorURL string
	adminURL     string
	adminSecret  string
	client       *http.Client
	issuer       localoidc.Issuer
	pool         *pgxpool.Pool
	root         string
}

func TestComposedBinariesServeAuthenticatedAPI(t *testing.T) {
	demo := startComposedDemo(t, "healthy.json")
	token, err := demo.issuer.Mint(localoidc.MintRequest{Subject: "p06-composed-admin", Roles: []access.Role{access.RoleAdmin}, TTL: 5 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodGet, demo.apiURL+"/v1/targets?page_size=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := demo.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("composed target read status=%d", response.StatusCode)
	}
}

func TestComposedBinariesCompleteHealthySixTargetCampaign(t *testing.T) {
	runComposedSixTargetCampaign(t, "healthy.json", "p06-a01", "P06-A01", "COMPLETED", "six target updates completed after public campaign validation and start")
}

func TestComposedBinariesRecoverLostEventByPolling(t *testing.T) {
	runComposedSixTargetCampaign(t, "lost-event.json", "p06-a02", "P06-A02", "COMPLETED", "lost simulator event did not prevent durable polling-based campaign completion")
}

func TestComposedBinariesAdoptPostAcceptDisconnectWithoutDuplicateSubmit(t *testing.T) {
	runComposedSixTargetCampaign(t, "post-accept-disconnect.json", "p06-a03", "P06-A03", "COMPLETED", "post-accept disconnect was adopted without duplicate backend acceptance")
}

func TestComposedBinariesPauseBeforeLaterWaveAfterCanaryFailure(t *testing.T) {
	runComposedSixTargetCampaign(t, "canary-health-failure.json", "p06-a04", "P06-A04", "PAUSED", "canary health failure durably paused the campaign before later-wave admission")
}

func TestComposedBinariesResolveRecentAdminAttentionWithReplayFence(t *testing.T) {
	runComposedSixTargetCampaign(t, "task-disappearance.json", "p06-a05", "P06-A05", "PAUSED", "recent administrator resolution was revision-fenced and idempotently replayed")
}

func runComposedSixTargetCampaign(t *testing.T, fixtureName, group, caseID, expectedState, invariant string) {
	t.Helper()
	demo := startComposedDemo(t, fixtureName)
	token, err := demo.issuer.Mint(localoidc.MintRequest{Subject: "p06-composed-admin", Roles: []access.Role{access.RoleAdmin}, TTL: 5 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(acceptanceRepositoryRoot(t), "test", "fixtures", "simulator", "v1", fixtureName)
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := scenario.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range fixture.Targets {
		key, err := scenario.ID(fixture, "target", target.Index)
		if err != nil {
			t.Fatal(err)
		}
		body := fmt.Sprintf(`{"display_name":"p06-%s","endpoint_reference":"%s","target_key":"%s","credential_reference":"synthetic-test:%s","labels":{"site":"%s","rack":"%s","family":"synthetic","group":"%s"}}`, key, demo.simulatorURL, key, key, target.Site, target.Rack, group)
		request := authenticatedRequest(t, http.MethodPost, demo.apiURL+"/v1/targets", body, token)
		request.Header.Set("Idempotency-Key", "p06-composed-register-"+key[:16])
		response, err := demo.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusCreated {
			response.Body.Close()
			t.Fatalf("register target status=%d", response.StatusCode)
		}
		var registered controlhttp.CommandEnvelope
		err = json.NewDecoder(response.Body).Decode(&registered)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		discover := authenticatedRequest(t, http.MethodPost, demo.apiURL+"/v1/targets/"+registered.Resource.Id.String()+":discover", "", token)
		discover.Header.Set("Idempotency-Key", "p06-composed-discover-"+key[:16])
		discover.Header.Set("If-Match", "\"rev-0\"")
		response, err = demo.client.Do(discover)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("schedule discovery status=%d", response.StatusCode)
		}
	}
	waitForComposedDiscovery(t, demo, len(fixture.Targets))
	artifactBytes := []byte("p06-healthy-firmware")
	digest := sha256.Sum256(artifactBytes)
	initiate := authenticatedRequest(t, http.MethodPost, demo.apiURL+"/v1/artifacts:initiate", `{"sha256":"`+hex.EncodeToString(digest[:])+`","size_bytes":20,"media_type":"application/octet-stream","firmware_version":"1.1"}`, token)
	initiate.Header.Set("Idempotency-Key", "p06-a01-artifact-initiate")
	response, err := demo.client.Do(initiate)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated {
		response.Body.Close()
		t.Fatalf("artifact initiation status=%d", response.StatusCode)
	}
	var artifact controlhttp.ArtifactInitiationEnvelope
	err = json.NewDecoder(response.Body).Decode(&artifact)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	upload, err := http.NewRequestWithContext(t.Context(), http.MethodPut, artifact.Upload.Url, bytes.NewReader(artifactBytes))
	if err != nil {
		t.Fatal(err)
	}
	upload.Header.Set("X-FleetForge-Upload-Grant", artifact.Upload.Grant)
	upload.Header.Set("Content-Type", "application/octet-stream")
	response, err = demo.client.Do(upload)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("artifact upload status=%d", response.StatusCode)
	}
	waitForCondition(t, 20*time.Second, func() (bool, error) {
		var state string
		err := demo.pool.QueryRow(t.Context(), "SELECT state FROM firmware_artifacts WHERE id=$1::uuid", artifact.Resource.Id.String()).Scan(&state)
		return state == "VERIFIED", err
	})
	operator, err := demo.issuer.Mint(localoidc.MintRequest{Subject: "p06-composed-operator", Roles: []access.Role{access.RoleOperator}, TTL: 5 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	create := authenticatedRequest(t, http.MethodPost, demo.apiURL+"/v1/campaigns", campaignCreateBody(artifact.Resource.Id.String(), group), operator)
	create.Header.Set("Idempotency-Key", group+"-campaign-create")
	response, err = demo.client.Do(create)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated {
		response.Body.Close()
		t.Fatalf("campaign creation status=%d", response.StatusCode)
	}
	var campaign controlhttp.CommandEnvelope
	err = json.NewDecoder(response.Body).Decode(&campaign)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	validate := authenticatedRequest(t, http.MethodPost, demo.apiURL+"/v1/campaigns/"+campaign.Resource.Id.String()+":validate", "", operator)
	validate.Header.Set("Idempotency-Key", group+"-validate")
	validate.Header.Set("If-Match", "\"rev-0\"")
	response, err = demo.client.Do(validate)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("campaign validation status=%d", response.StatusCode)
	}
	readyETag := waitForCampaignState(t, demo, operator, campaign.Resource.Id.String(), "READY", 30*time.Second)
	start := authenticatedRequest(t, http.MethodPost, demo.apiURL+"/v1/campaigns/"+campaign.Resource.Id.String()+":start", "", operator)
	start.Header.Set("Idempotency-Key", group+"-campaign-start")
	start.Header.Set("If-Match", readyETag)
	response, err = demo.client.Do(start)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("campaign start status=%d", response.StatusCode)
	}
	waitForCampaignState(t, demo, operator, campaign.Resource.Id.String(), expectedState, 180*time.Second)
	var targets, succeeded int
	if err := demo.pool.QueryRow(t.Context(), "SELECT count(*),count(*) FILTER (WHERE tu.state='SUCCEEDED') FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id WHERE ct.campaign_id=$1::uuid", campaign.Resource.Id.String()).Scan(&targets, &succeeded); err != nil {
		t.Fatal(err)
	}
	if expectedState == "COMPLETED" && (targets != 6 || succeeded != targets) {
		t.Fatalf("terminal target updates targets=%d succeeded=%d", targets, succeeded)
	}
	if caseID == "P06-A02" && readSimulatorMetrics(t, demo).Faults == 0 {
		t.Fatal("lost-event fixture completed without consuming its simulator fault")
	}
	if caseID == "P06-A03" {
		metrics := readSimulatorMetrics(t, demo)
		if metrics.Faults == 0 || metrics.AcceptedUpdates == 0 || metrics.AcceptedUpdates > uint64(targets) {
			t.Fatalf("post-accept metrics faults=%d accepted_updates=%d targets=%d", metrics.Faults, metrics.AcceptedUpdates, targets)
		}
	}
	if caseID == "P06-A04" {
		metrics := readSimulatorMetrics(t, demo)
		var admittedLater int
		if err := demo.pool.QueryRow(t.Context(), "SELECT count(*) FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id WHERE ct.campaign_id=$1::uuid AND ct.cohort='LATER_WAVE' AND tu.state <> 'PENDING'", campaign.Resource.Id.String()).Scan(&admittedLater); err != nil || metrics.Faults == 0 || admittedLater != 0 {
			t.Fatalf("canary pause metrics faults=%d later_admitted=%d err=%v", metrics.Faults, admittedLater, err)
		}
	}
	if caseID == "P06-A05" {
		var targetUpdateID string
		var revision int64
		waitForCondition(t, 20*time.Second, func() (bool, error) {
			err := demo.pool.QueryRow(t.Context(), "SELECT tu.id::text,tu.revision FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id WHERE ct.campaign_id=$1::uuid AND tu.state='NEEDS_ATTENTION' ORDER BY tu.id LIMIT 1", campaign.Resource.Id.String()).Scan(&targetUpdateID, &revision)
			if err != nil {
				return false, nil
			}
			return true, nil
		})
		resolve := authenticatedRequest(t, http.MethodPost, demo.apiURL+"/v1/target-updates/"+targetUpdateID+":resolve-attention", `{"action":"TERMINAL","reason_code":"operator.reviewed","reason":"verified","reservation_disposition":"QUARANTINE"}`, token)
		resolve.Header.Set("Idempotency-Key", "p06-a05-resolve-attention")
		resolve.Header.Set("If-Match", fmt.Sprintf("\"rev-%d\"", revision))
		response, err := demo.client.Do(resolve)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("attention resolution status=%d", response.StatusCode)
		}
		replay := authenticatedRequest(t, http.MethodPost, demo.apiURL+"/v1/target-updates/"+targetUpdateID+":resolve-attention", `{"action":"TERMINAL","reason_code":"operator.reviewed","reason":"verified","reservation_disposition":"QUARANTINE"}`, token)
		replay.Header.Set("Idempotency-Key", "p06-a05-resolve-attention")
		replay.Header.Set("If-Match", fmt.Sprintf("\"rev-%d\"", revision))
		response, err = demo.client.Do(replay)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("attention replay status=%d", response.StatusCode)
		}
		var resolutions int
		if err := demo.pool.QueryRow(t.Context(), "SELECT count(*) FROM ambiguity_resolutions ar JOIN firmware_update_attempts fa ON fa.id=ar.attempt_id WHERE fa.target_update_id=$1::uuid", targetUpdateID).Scan(&resolutions); err != nil || resolutions != 1 {
			t.Fatalf("attention resolutions=%d err=%v", resolutions, err)
		}
	}
	recordP06Evidence(t, caseID, fixture.Seed, fixturePath, invariant, map[string]int64{"target_updates": int64(targets), "succeeded_updates": int64(succeeded)}, map[string]string{"artifact": "sha256:" + hex.EncodeToString(digest[:])})
}

func startComposedDemo(t *testing.T, fixtureName string) *composedDemo {
	t.Helper()
	harness, pool, _, repositoryRoot := acceptanceHarness(t)
	fixturePath := filepath.Join(repositoryRoot, "test", "fixtures", "simulator", "v1", fixtureName)
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := scenario.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	runID, err := scenario.ID(fixture, "run", 0)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	addresses := map[string]string{
		"api":       loopbackAddress(t),
		"object":    loopbackAddress(t),
		"simulator": loopbackAddress(t),
		"admin":     loopbackAddress(t),
	}
	key := []byte("01234567890123456789012345678901")
	encodedKey := base64.RawStdEncoding.EncodeToString(key)
	credentials := make(map[string]map[string]string, len(fixture.Targets))
	for _, target := range fixture.Targets {
		id, err := scenario.ID(fixture, "target", target.Index)
		if err != nil {
			t.Fatal(err)
		}
		credentials[id] = map[string]string{"username": "p06-user", "password": "p06-password"}
	}
	credentialsJSON, err := json.Marshal(credentials)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"FLEETFORGE_ENV":                         "test",
		"FLEETFORGE_LOG_LEVEL":                   "warn",
		"FLEETFORGE_SHUTDOWN_TIMEOUT":            "10s",
		"FLEETFORGE_READINESS_TIMEOUT":           "500ms",
		"FLEETFORGE_POSTGRES_APP_DSN":            harness.ScopedDSN(t),
		"FLEETFORGE_POSTGRES_MIGRATION_DSN":      harness.ScopedDSN(t),
		"FLEETFORGE_POSTGRES_MAX_CONNS":          "2",
		"FLEETFORGE_API_ADDR":                    addresses["api"],
		"FLEETFORGE_OBJECT_STORE_ADDR":           addresses["object"],
		"FLEETFORGE_OBJECT_STORE_ROOT":           filepath.Join(root, "objects"),
		"FLEETFORGE_OBJECT_STORE_HMAC_SECRET":    encodedKey,
		"FLEETFORGE_API_CURSOR_HMAC_SECRET":      encodedKey,
		"FLEETFORGE_LOCAL_AUTH_ISSUER":           "fleetforge-p06-acceptance",
		"FLEETFORGE_LOCAL_AUTH_AUDIENCE":         "fleetforge-p06-acceptance",
		"FLEETFORGE_LOCAL_AUTH_KEY":              encodedKey,
		"FLEETFORGE_SIMULATOR_ADDR":              addresses["simulator"],
		"FLEETFORGE_SIMULATOR_ADMIN_ADDR":        addresses["admin"],
		"FLEETFORGE_SIMULATOR_RUN_ROOT":          filepath.Join(root, "simulator-runs"),
		"FLEETFORGE_SIMULATOR_SCENARIO_PATH":     fixturePath,
		"FLEETFORGE_SIMULATOR_USERNAME":          "p06-user",
		"FLEETFORGE_SIMULATOR_PASSWORD":          "p06-password",
		"FLEETFORGE_SIMULATOR_ADMIN_SECRET":      "p06-admin-secret",
		"FLEETFORGE_DISCOVERY_ALLOWED_CIDRS":     "127.0.0.0/8",
		"FLEETFORGE_DISCOVERY_CREDENTIALS_JSON":  string(credentialsJSON),
		"FLEETFORGE_PRECHECK_ALLOWED_CIDRS":      "127.0.0.0/8",
		"FLEETFORGE_PRECHECK_CREDENTIALS_JSON":   string(credentialsJSON),
		"FLEETFORGE_EXECUTION_ALLOWED_CIDRS":     "127.0.0.0/8",
		"FLEETFORGE_EXECUTION_CREDENTIALS_JSON":  string(credentialsJSON),
		"FLEETFORGE_RECONCILIATION_JOURNAL_PATH": filepath.Join(root, "simulator-runs", runID, "journal.jsonl"),
		"FLEETFORGE_RECONCILIATION_RUN_ID":       runID,
		"FLEETFORGE_WAKEUP_RECEIVE_BATCH":        "1",
		"FLEETFORGE_WAKEUP_WORKER_CONCURRENCY":   "1",
		"FLEETFORGE_WAKEUP_PUBLISHER_BATCH":      "4",
		"FLEETFORGE_WAKEUP_LOCAL_QUEUE_CAPACITY": "8",
		"FLEETFORGE_WAKEUP_HANDLER_TIMEOUT":      "5s",
		"FLEETFORGE_WAKEUP_LEASE_DURATION":       "10s",
		"FLEETFORGE_WAKEUP_PUBLISHER_LEASE":      "10s",
		"FLEETFORGE_WAKEUP_REPAIR_GRACE":         "1s",
		"FLEETFORGE_WAKEUP_MAX_BACKOFF":          "1s",
		"FLEETFORGE_WAKEUP_RETRY_ATTEMPTS":       "2",
	}
	for _, name := range []string{"local-object-store", "redfish-simulator"} {
		startDemoProcess(t, repositoryRoot, root, name, env)
	}
	waitForReady(t, "http://"+addresses["simulator"]+"/health/ready")
	for _, name := range []string{"orchestrator-worker", "discovery-worker", "wakeup-worker", "precheck-worker", "execution-worker", "observation-worker", "reconciliation-worker", "control-api"} {
		startDemoProcess(t, repositoryRoot, root, name, env)
	}
	apiURL := "http://" + addresses["api"]
	waitForReady(t, apiURL+"/health/ready")
	return &composedDemo{apiURL: apiURL, simulatorURL: "http://" + addresses["simulator"], adminURL: "http://" + addresses["admin"], adminSecret: env["FLEETFORGE_SIMULATOR_ADMIN_SECRET"], client: &http.Client{Timeout: 5 * time.Second}, issuer: localoidc.Issuer{Issuer: env["FLEETFORGE_LOCAL_AUTH_ISSUER"], Audience: env["FLEETFORGE_LOCAL_AUTH_AUDIENCE"], Key: key}, pool: pool, root: root}
}
func loopbackAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func startDemoProcess(t *testing.T, repositoryRoot, root, name string, values map[string]string) {
	t.Helper()
	binary := filepath.Join(root, "bin", name)
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(binary), 0o700); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/"+name)
	build.Dir = repositoryRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v: %s", name, err, redactProcessOutput(string(output)))
	}
	logFile, err := os.OpenFile(filepath.Join(root, name+".log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary)
	command.Dir = repositoryRoot
	command.Env = append(os.Environ(), flattenedEnvironment(values)...)
	command.Stdout, command.Stderr = logFile, logFile
	if err := command.Start(); err != nil {
		logFile.Close()
		t.Fatalf("start %s: %v", name, err)
	}
	t.Cleanup(func() {
		if command.Process != nil {
			_ = command.Process.Kill()
			_, _ = command.Process.Wait()
		}
		_ = logFile.Close()
	})
}

func flattenedEnvironment(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

func waitForReady(t *testing.T, endpoint string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(endpoint) // #nosec G107 -- endpoint is a test-owned loopback listener.
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for composed control API")
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ready, err := condition()
		if err != nil {
			t.Fatal(err)
		}
		if ready {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for bounded composed-runtime condition")
}

func waitForComposedDiscovery(t *testing.T, demo *composedDemo, want int) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err := demo.pool.QueryRow(t.Context(), "SELECT count(*) FROM target_discovery_heads WHERE last_good_id IS NOT NULL").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	var good, failed, pending, claimed, completed int
	var failureKinds string
	err := demo.pool.QueryRow(t.Context(), `SELECT
	(SELECT count(*) FROM target_discovery_heads WHERE last_good_id IS NOT NULL),
	(SELECT count(*) FROM device_observations WHERE kind='DISCOVERY_FAILURE'),
	(SELECT count(*) FROM due_work WHERE work_type='DISCOVER' AND delivery_state='PENDING'),
	(SELECT count(*) FROM due_work WHERE work_type='DISCOVER' AND delivery_state='CLAIMED'),
	(SELECT count(*) FROM due_work WHERE work_type='DISCOVER' AND delivery_state='COMPLETED'),
	COALESCE((SELECT string_agg(error_kind || ':' || count::text, ',' ORDER BY error_kind) FROM (SELECT normalized_data->>'error_kind' AS error_kind,count(*) FROM device_observations WHERE kind='DISCOVERY_FAILURE' GROUP BY normalized_data->>'error_kind') failures), '')`).Scan(&good, &failed, &pending, &claimed, &completed, &failureKinds)
	t.Fatalf("composed discovery timed out good=%d failed=%d pending=%d claimed=%d completed=%d kinds=%s err=%v", good, failed, pending, claimed, completed, failureKinds, err)
}

func waitForCampaignState(t *testing.T, demo *composedDemo, token, campaignID, expected string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		request := authenticatedRequest(t, http.MethodGet, demo.apiURL+"/v1/campaigns/"+campaignID, "", token)
		response, err := demo.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			t.Fatalf("campaign projection status=%d", response.StatusCode)
		}
		var campaign controlhttp.Campaign
		err = json.NewDecoder(response.Body).Decode(&campaign)
		etag := response.Header.Get("ETag")
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if campaign.State == expected {
			if etag == "" {
				t.Fatal("campaign projection missing ETag")
			}
			return etag
		}
		time.Sleep(200 * time.Millisecond)
	}
	var state string
	var updates, succeeded, terminal, pendingWork, claimedWork int
	var claimedBy, updateStates, attemptStates, lastTargetAudit, workRows, taskObservation string
	err := demo.pool.QueryRow(t.Context(), `SELECT
	c.state,
	(SELECT count(*) FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id WHERE ct.campaign_id=c.id),
	(SELECT count(*) FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id WHERE ct.campaign_id=c.id AND tu.state='SUCCEEDED'),
	(SELECT count(*) FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id WHERE ct.campaign_id=c.id AND tu.state IN ('SUCCEEDED','FAILED_TERMINAL','SKIPPED','CANCELLED','NEEDS_ATTENTION')),
	(SELECT count(*) FROM due_work dw JOIN target_updates tu ON tu.id=dw.subject_id JOIN campaign_targets ct ON ct.id=tu.campaign_target_id WHERE dw.delivery_state='PENDING' AND ct.campaign_id=c.id),
	(SELECT count(*) FROM due_work dw JOIN target_updates tu ON tu.id=dw.subject_id JOIN campaign_targets ct ON ct.id=tu.campaign_target_id WHERE dw.delivery_state='CLAIMED' AND ct.campaign_id=c.id),
	COALESCE((SELECT string_agg(dw.work_type || ':' || COALESCE(dw.lease_owner,''), ',' ORDER BY dw.work_type,dw.lease_owner) FROM due_work dw JOIN target_updates tu ON tu.id=dw.subject_id JOIN campaign_targets ct ON ct.id=tu.campaign_target_id WHERE dw.delivery_state='CLAIMED' AND ct.campaign_id=c.id), '')
FROM campaigns c WHERE c.id=$1::uuid`, campaignID).Scan(&state, &updates, &succeeded, &terminal, &pendingWork, &claimedWork, &claimedBy)
	_ = demo.pool.QueryRow(t.Context(), `SELECT COALESCE(string_agg(tu.state || ':' || COALESCE(tu.resume_step,'') || ':' || COALESCE(a.status,'') || ':' || COALESCE(a.result_class,'') || ':fp=' || CASE WHEN a.response_fingerprint IS NULL THEN '0' ELSE '1' END || ':task=' || CASE WHEN a.remote_task_reference IS NULL THEN '0' ELSE '1' END, ',' ORDER BY tu.state,tu.id),'') FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id LEFT JOIN firmware_update_attempts a ON a.target_update_id=tu.id WHERE ct.campaign_id=$1::uuid`, campaignID).Scan(&attemptStates)
	_ = demo.pool.QueryRow(t.Context(), `SELECT COALESCE(action || ':' || reason_code,'') FROM audit_events WHERE aggregate_type='TARGET_UPDATE' AND aggregate_id IN (SELECT tu.id FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id WHERE ct.campaign_id=$1::uuid) ORDER BY created_at DESC,id DESC LIMIT 1`, campaignID).Scan(&lastTargetAudit)
	_ = demo.pool.QueryRow(t.Context(), `SELECT COALESCE(string_agg(dw.work_type || ':' || dw.delivery_state || ':due=' || CASE WHEN dw.not_before <= now() THEN '1' ELSE '0' END || ':at=' || to_char(dw.not_before AT TIME ZONE 'UTC','HH24:MI:SS') || ':now=' || to_char(now() AT TIME ZONE 'UTC','HH24:MI:SS'), ',' ORDER BY dw.work_type,dw.id),'') FROM due_work dw JOIN target_updates tu ON tu.id=dw.subject_id JOIN campaign_targets ct ON ct.id=tu.campaign_target_id WHERE ct.campaign_id=$1::uuid`, campaignID).Scan(&workRows)
	_ = demo.pool.QueryRow(t.Context(), `SELECT COALESCE(normalized_data::text,'') FROM device_observations o JOIN target_updates tu ON tu.id=o.target_update_id JOIN campaign_targets ct ON ct.id=tu.campaign_target_id WHERE ct.campaign_id=$1::uuid AND o.kind='TASK' ORDER BY o.received_at DESC,o.id DESC LIMIT 1`, campaignID).Scan(&taskObservation)
	t.Fatalf("timed out waiting for campaign state %s state=%s updates=%d succeeded=%d terminal=%d states=%s attempts=%s last_target_audit=%s pending_work=%d claimed_work=%d claimed_by=%s work_rows=%s task_observation=%s wakeup=%s precheck=%s execution=%s observation=%s err=%v", expected, state, updates, succeeded, terminal, updateStates, attemptStates, lastTargetAudit, pendingWork, claimedWork, claimedBy, workRows, taskObservation, wakeupFailureKind(demo.root), precheckFailureKind(demo.root), executionFailureKind(demo.root), observationFailureKind(demo.root), err)
	return ""
}

type simulatorMetrics struct {
	AcceptedUpdates uint64 `json:"AcceptedUpdates"`
	Faults          uint64 `json:"Faults"`
}

func readSimulatorMetrics(t *testing.T, demo *composedDemo) simulatorMetrics {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, demo.adminURL+"/admin/v1/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-FleetForge-Simulator-Admin", demo.adminSecret)
	response, err := demo.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("simulator metrics status=%d", response.StatusCode)
	}
	var metrics simulatorMetrics
	if err := json.NewDecoder(response.Body).Decode(&metrics); err != nil {
		t.Fatal(err)
	}
	return metrics
}

func wakeupFailureKind(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, "wakeup-worker.log"))
	if err != nil {
		return "unavailable"
	}
	log := string(raw)
	switch {
	case strings.Contains(log, "context deadline exceeded"):
		return "deadline"
	case strings.Contains(log, "active enrollment unavailable"):
		return "enrollment_fence"
	case strings.Contains(log, "lease conflict"):
		return "lease_conflict"
	case strings.Contains(log, "lock firmware precheck start"):
		return "precheck_lock"
	case strings.Contains(log, "process wakeups"):
		return "wakeup_failure"
	default:
		return "unknown"
	}
}

func precheckFailureKind(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, "precheck-worker.log"))
	if err != nil {
		return "unavailable"
	}
	log := string(raw)
	switch {
	case strings.Contains(log, "load precheck command"):
		return "command"
	case strings.Contains(log, "load fenced precheck"):
		return "fence"
	case strings.Contains(log, "collect target precheck"):
		return "target"
	case strings.Contains(log, "persist firmware admission"):
		return "admission"
	case strings.Contains(log, "invalid frozen campaign policy"):
		return "policy"
	case strings.Contains(log, "firmware precheck cycle failed"):
		return "unknown"
	default:
		return "none"
	}
}

func redactProcessOutput(string) string { return "child process output redacted" }

func executionFailureKind(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, "execution-worker.log"))
	if err != nil {
		return "unavailable"
	}
	log := string(raw)
	switch {
	case strings.Contains(log, "open exact artifact"):
		return "artifact"
	case strings.Contains(log, "load immutable attempt"):
		return "attempt"
	case strings.Contains(log, "target returned result"):
		return "target_result"
	case strings.Contains(log, "commit submit result"):
		return "commit"
	case strings.Contains(log, "firmware submit cycle failed"):
		return "unknown"
	default:
		return "none"
	}
}

func observationFailureKind(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, "observation-worker.log"))
	if err != nil {
		return "unavailable"
	}
	log := string(raw)
	switch {
	case strings.Contains(log, "observation worker stopped"):
		return "startup"
	case strings.Contains(log, "load fenced observation attempt"):
		return "attempt"
	case strings.Contains(log, "collect Redfish task"):
		return "task"
	case strings.Contains(log, "commit observation"):
		return "commit"
	case strings.Contains(log, "observation cycle failed"):
		return "unknown"
	default:
		return "none"
	}
}
