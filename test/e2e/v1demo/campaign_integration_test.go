//go:build postgresintegration

package v1demo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parth10-05/fleetforge/internal/application/access"
	artifactapp "github.com/parth10-05/fleetforge/internal/application/artifacts"
	validationapp "github.com/parth10-05/fleetforge/internal/application/campaignvalidation"
	"github.com/parth10-05/fleetforge/internal/application/execution"
	appinventory "github.com/parth10-05/fleetforge/internal/application/inventory"
	"github.com/parth10-05/fleetforge/internal/application/orchestration"
	preflightapp "github.com/parth10-05/fleetforge/internal/application/preflight"
	"github.com/parth10-05/fleetforge/internal/delivery/controlhttp"
	"github.com/parth10-05/fleetforge/internal/delivery/localobjecthttp"
	"github.com/parth10-05/fleetforge/internal/delivery/redfishsim"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
	"github.com/parth10-05/fleetforge/internal/platform/localwakeup"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticdiscovery"
	"github.com/parth10-05/fleetforge/internal/redfish"
	"github.com/parth10-05/fleetforge/internal/simulator/core"
	"github.com/parth10-05/fleetforge/internal/simulator/journal"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

func TestPublicCampaignValidatesAndStarts(t *testing.T) {
	_, pool, ctx, root := acceptanceHarness(t)
	fixturePath := filepath.Join(root, "test", "fixtures", "simulator", "v1", "healthy.json")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := scenario.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := core.Start(fixture, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	writer, err := journal.OpenWriter(filepath.Join(t.TempDir(), "journal.jsonl"), "p06-campaign")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	simulator := httptest.NewServer(redfishsim.NewWithJournal(engine, []byte("user"), []byte("pass"), time.Second, 1024, writer).Routes())
	defer simulator.Close()

	secret := []byte("01234567890123456789012345678901")
	store, err := localartifactstore.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	completion := artifactapp.NewWithCompletion(nil, nil, store, persistence.ArtifactBindingAdapter{Pool: pool}, nil)
	upload := httptest.NewServer(localobjecthttp.New(store, secret, time.Now).WithCompletion(completion).Routes())
	defer upload.Close()
	api := newAcceptanceAPI(t, pool, upload.URL+"/v1/objects/upload")
	admin, operator := api.token(t, access.RoleAdmin), api.token(t, access.RoleOperator)
	credential, err := syntheticcredentials.NewCredential("user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	credentials := make(map[string]syntheticcredentials.Credential, len(fixture.Targets))
	targetIDs := make([]string, 0, len(fixture.Targets))
	for _, fixtureTarget := range fixture.Targets {
		targetKey, err := scenario.ID(fixture, "target", fixtureTarget.Index)
		if err != nil {
			t.Fatal(err)
		}
		credentials[targetKey] = credential
		register := authenticatedRequest(t, http.MethodPost, api.server.URL+"/v1/targets", `{"display_name":"acceptance-campaign-`+targetKey+`","endpoint_reference":"`+simulator.URL+`","target_key":"`+targetKey+`","credential_reference":"synthetic-test:`+targetKey+`","labels":{"site":"`+fixtureTarget.Site+`","rack":"`+fixtureTarget.Rack+`","family":"synthetic","group":"p06-a01"}}`, admin)
		register.Header.Set("Idempotency-Key", "p06-campaign-register-"+targetKey)
		registered, err := api.server.Client().Do(register)
		if err != nil {
			t.Fatal(err)
		}
		if registered.StatusCode != http.StatusCreated {
			registered.Body.Close()
			t.Fatalf("registration status=%d", registered.StatusCode)
		}
		var target controlhttp.CommandEnvelope
		err = json.NewDecoder(registered.Body).Decode(&target)
		registered.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		targetIDs = append(targetIDs, target.Resource.Id.String())
		discover := authenticatedRequest(t, http.MethodPost, api.server.URL+"/v1/targets/"+target.Resource.Id.String()+":discover", "", admin)
		discover.Header.Set("Idempotency-Key", "p06-campaign-discover-"+targetKey)
		discover.Header.Set("If-Match", "\"rev-0\"")
		accepted, err := api.server.Client().Do(discover)
		if err != nil {
			t.Fatal(err)
		}
		if accepted.StatusCode != http.StatusAccepted {
			accepted.Body.Close()
			t.Fatalf("discovery status=%d", accepted.StatusCode)
		}
		accepted.Body.Close()
	}
	resolver, err := syntheticcredentials.New(credentials)
	if err != nil {
		t.Fatal(err)
	}
	discoveryWork := persistence.DiscoveryWorkAdapter{Pool: pool}
	discoveryExecutor := syntheticdiscovery.Executor{Loader: discoveryWork, Client: redfish.NewClient(redfish.ClientConfig{HTTPClient: simulator.Client(), AllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}), Resolver: resolver}
	now := time.Now().UTC()
	for range targetIDs {
		if processed, err := appinventory.RunDiscoveryOnce(context.Background(), discoveryWork, discoveryExecutor, discoveryWork, appinventory.DiscoveryClaimRequest{Owner: "p06-acceptance", LeaseToken: uuid.NewString(), Now: now, LeaseExpiresAt: now.Add(time.Minute)}, func(_ appinventory.DiscoveryWork, at time.Time) time.Time { return at.Add(time.Second) }); err != nil || !processed {
			t.Fatalf("run discovery processed=%t err=%v", processed, err)
		}
	}

	bytesToUpload := []byte("abc")
	digest := sha256.Sum256(bytesToUpload)
	initiate := authenticatedRequest(t, http.MethodPost, api.server.URL+"/v1/artifacts:initiate", `{"sha256":"`+hex.EncodeToString(digest[:])+`","size_bytes":3,"media_type":"application/octet-stream","firmware_version":"1.1"}`, admin)
	initiate.Header.Set("Idempotency-Key", "p06-campaign-artifact-0001")
	initiatedResponse, err := api.server.Client().Do(initiate)
	if err != nil {
		t.Fatal(err)
	}
	defer initiatedResponse.Body.Close()
	if initiatedResponse.StatusCode != http.StatusCreated {
		t.Fatalf("artifact initiation status=%d", initiatedResponse.StatusCode)
	}
	var artifactResult controlhttp.ArtifactInitiationEnvelope
	if err := json.NewDecoder(initiatedResponse.Body).Decode(&artifactResult); err != nil {
		t.Fatal(err)
	}
	uploadRequest, err := http.NewRequestWithContext(ctx, http.MethodPut, artifactResult.Upload.Url, bytes.NewReader(bytesToUpload))
	if err != nil {
		t.Fatal(err)
	}
	uploadRequest.Header.Set(localobjecthttp.GrantHeader, artifactResult.Upload.Grant)
	uploadRequest.Header.Set("Content-Type", "application/octet-stream")
	uploadResponse, err := upload.Client().Do(uploadRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer uploadResponse.Body.Close()
	if uploadResponse.StatusCode != http.StatusCreated {
		t.Fatalf("artifact upload status=%d", uploadResponse.StatusCode)
	}
	verification := persistence.ArtifactVerificationAdapter{Pool: pool}
	artifactWorker := artifactapp.NewWorker(artifactapp.NewWithCompletion(nil, nil, store, nil, nil).WithVerification(verification, verification), persistence.ArtifactVerificationWorkAdapter{Pool: pool}, persistence.ArtifactVerificationWorkAdapter{Pool: pool}, time.Now)
	now = time.Now().UTC()
	if err := artifactWorker.RunOnce(ctx, artifactapp.VerificationWorkClaimRequest{Owner: "p06-acceptance", LeaseToken: uuid.NewString(), Now: now, LeaseExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}

	create := authenticatedRequest(t, http.MethodPost, api.server.URL+"/v1/campaigns", campaignCreateBody(artifactResult.Resource.Id.String(), "p06-a01"), operator)
	create.Header.Set("Idempotency-Key", "p06-campaign-create-0001")
	created, err := api.server.Client().Do(create)
	if err != nil {
		t.Fatal(err)
	}
	defer created.Body.Close()
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("campaign creation status=%d", created.StatusCode)
	}
	var campaign controlhttp.CommandEnvelope
	if err := json.NewDecoder(created.Body).Decode(&campaign); err != nil {
		t.Fatal(err)
	}
	validate := authenticatedRequest(t, http.MethodPost, api.server.URL+"/v1/campaigns/"+campaign.Resource.Id.String()+":validate", "", operator)
	validate.Header.Set("Idempotency-Key", "p06-campaign-validate-0001")
	validate.Header.Set("If-Match", "\"rev-0\"")
	validated, err := api.server.Client().Do(validate)
	if err != nil {
		t.Fatal(err)
	}
	defer validated.Body.Close()
	if validated.StatusCode != http.StatusAccepted {
		t.Fatalf("validation status=%d", validated.StatusCode)
	}
	work := persistence.CampaignValidationWorkAdapter{Pool: pool}
	preflight := persistence.PreflightAdapter{Pool: pool}
	worker := validationapp.NewWorker(work, work, validationapp.NewValidationProcessor(persistence.CampaignValidationInputAdapter{Pool: pool}, preflightapp.New(preflight, preflight, nil), persistence.CampaignValidationFinalizer{Pool: pool}, time.Now), time.Now)
	now = time.Now().UTC()
	if processed, err := worker.RunOnce(ctx, validationapp.ClaimRequest{Owner: "p06-acceptance", LeaseToken: uuid.NewString(), Now: now, LeaseExpiresAt: now.Add(time.Minute)}); err != nil || !processed {
		t.Fatalf("run validation processed=%t err=%v", processed, err)
	}
	projectionRequest := authenticatedRequest(t, http.MethodGet, api.server.URL+"/v1/campaigns/"+campaign.Resource.Id.String(), "", operator)
	projectionResponse, err := api.server.Client().Do(projectionRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer projectionResponse.Body.Close()
	if projectionResponse.StatusCode != http.StatusOK {
		t.Fatalf("campaign projection status=%d", projectionResponse.StatusCode)
	}
	var projection controlhttp.Campaign
	if err := json.NewDecoder(projectionResponse.Body).Decode(&projection); err != nil {
		t.Fatal(err)
	}
	if projection.State != "READY" || projection.Revision != 2 {
		t.Fatalf("campaign=%s/%d", projection.State, projection.Revision)
	}
	var planTargets, campaignTargets, immutablePlans int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM campaign_targets WHERE campaign_id=$1::uuid", campaign.Resource.Id.String()).Scan(&campaignTargets); err != nil || campaignTargets != len(targetIDs) {
		t.Fatalf("campaign targets=%d err=%v", campaignTargets, err)
	}
	if err := pool.QueryRow(ctx, "SELECT target_count FROM preflight_candidates WHERE artifact_id=$1::uuid", artifactResult.Resource.Id.String()).Scan(&planTargets); err != nil || planTargets != len(targetIDs) {
		t.Fatalf("preflight targets=%d err=%v", planTargets, err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM campaign_plan_snapshots WHERE campaign_id=$1::uuid", campaign.Resource.Id.String()).Scan(&immutablePlans); err != nil || immutablePlans != 1 {
		t.Fatalf("immutable plans=%d err=%v", immutablePlans, err)
	}
	start := authenticatedRequest(t, http.MethodPost, api.server.URL+"/v1/campaigns/"+campaign.Resource.Id.String()+":start", "", operator)
	start.Header.Set("Idempotency-Key", "p06-campaign-start-0001")
	start.Header.Set("If-Match", projectionResponse.Header.Get("ETag"))
	started, err := api.server.Client().Do(start)
	if err != nil {
		t.Fatal(err)
	}
	defer started.Body.Close()
	if started.StatusCode != http.StatusAccepted {
		t.Fatalf("campaign start status=%d", started.StatusCode)
	}
	var canaryWork int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM due_work WHERE subject_type='TARGET_UPDATE' AND work_type='CANARY_START' AND delivery_state='PENDING'").Scan(&canaryWork); err != nil || canaryWork != 1 {
		t.Fatalf("canary work=%d err=%v", canaryWork, err)
	}
	queue, err := localwakeup.New(localwakeup.Config{Capacity: 8, Clock: orchestration.ClockFunc(time.Now)})
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	now = time.Now().UTC()
	if err := persistence.PublishClaimedWakeups(ctx, pool, queue, persistence.ClaimOutboxRequest{Owner: "p06-acceptance", Now: now, LeaseExpiresAt: now.Add(time.Minute), Tokens: []string{uuid.NewString()}}, 3, func(_ persistence.ClaimedWakeup, at time.Time) time.Time { return at.Add(time.Second) }); err != nil {
		t.Fatal(err)
	}
	wakeupStore := persistence.WakeupWorkAdapter{Pool: pool, Owner: "p06-acceptance", LeaseDuration: time.Minute, Now: time.Now, NewToken: func() (string, error) { return uuid.NewString(), nil }}
	handlers, err := orchestration.NewMultiplexHandler(nil, []orchestration.FinalizingWorkHandler{execution.WakeupPrecheckStartHandler{Repository: persistence.ExecutionAdapter{Pool: pool}}})
	if err != nil {
		t.Fatal(err)
	}
	wakeupWorker, err := orchestration.NewWorker(queue, queue, wakeupStore, handlers, orchestration.WorkerConfig{ReceiveBatch: 1, Concurrency: 1, HandlerTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := wakeupWorker.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var precheckWork int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM due_work WHERE subject_type='TARGET_UPDATE' AND work_type='PRECHECK' AND delivery_state='PENDING'").Scan(&precheckWork); err != nil || precheckWork != 1 {
		t.Fatalf("precheck work=%d err=%v", precheckWork, err)
	}
}

func campaignCreateBody(artifactID, group string) string {
	return `{"artifact_id":"` + artifactID + `","selector":{"version":"fleetforge.selector/v1","all":[{"key":"group","value":"` + group + `"}]},"selection_seed":"p06-a01","policy":{"version":"fleetforge.campaign-policy/v1","canary":{"unit":"absolute","value":1},"limits":{"global":1,"site":1,"rack":1},"failure_budget":{"numerator":1,"denominator":1},"expected_firmware_version":"1.1","required_health_checks":["boot"],"maintenance":{"timezone":"UTC","weekdays":[1],"start_minute":1,"end_minute":2},"maximum_duration_seconds":60,"retry_budget":1,"observation_window_seconds":30,"observation_every_seconds":10,"observation_freshness_seconds":10,"governance_id":"p06-a01","strict_worst_case_capacity":false}}`
}
