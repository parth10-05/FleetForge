//go:build postgresintegration

package postgres

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
	"github.com/parth10-05/fleetforge/internal/delivery/controlhttp"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestControlCampaignRoutePersistsImmutableDraftReplay(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	artifactID := "00000000-0000-0000-0000-000000000601"
	mustExecute(t, pool, ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state) VALUES ('"+artifactID+"','v1','sha256:"+strings.Repeat("a", 64)+"','artifact.bin','VERIFIED')")
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	service := control.NewCampaignService(persistence.ControlCampaignAdapter{Pool: pool}, func() time.Time { return now })
	verifier := integrationVerifier{principal: access.Principal{Subject: "user:operator", Roles: []access.Role{access.RoleOperator}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}}
	router := controlhttp.NewRouterWithResourcesUploadAndCampaigns(verifier, func() time.Time { return now }, nil, nil, service, nil, "")
	body := `{"artifact_id":"` + artifactID + `","selector":{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"west"}]},"selection_seed":"seed","policy":{"version":"fleetforge.campaign-policy/v1","canary":{"unit":"absolute","value":1},"limits":{"global":1,"site":1,"rack":1},"failure_budget":{"numerator":1,"denominator":1},"expected_firmware_version":"v1","required_health_checks":["boot"],"maintenance":{"timezone":"UTC","weekdays":[1],"start_minute":1,"end_minute":2},"maximum_duration_seconds":60,"retry_budget":1,"observation_window_seconds":30,"observation_every_seconds":10,"observation_freshness_seconds":10,"governance_id":"change-1","strict_worst_case_capacity":false}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/campaigns", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer operator")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "campaign-create-0001")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response controlhttp.CommandEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	replay := httptest.NewRequest(http.MethodPost, "/v1/campaigns", strings.NewReader(body))
	replay.Header.Set("Authorization", "Bearer operator")
	replay.Header.Set("Content-Type", "application/json")
	replay.Header.Set("Idempotency-Key", "campaign-create-0001")
	replayRecorder := httptest.NewRecorder()
	router.ServeHTTP(replayRecorder, replay)
	if replayRecorder.Code != http.StatusCreated || replayRecorder.Body.String() != recorder.Body.String() {
		t.Fatalf("replay status=%d body=%s want=%s", replayRecorder.Code, replayRecorder.Body.String(), recorder.Body.String())
	}
	for _, table := range []string{"campaigns", "campaign_drafts", "audit_events", "decision_inputs", "outbox_events"} {
		var count int
		query := "SELECT COUNT(*) FROM " + table
		if table != "campaigns" && table != "campaign_drafts" {
			query += " WHERE aggregate_id=$1::uuid"
		} else {
			query += " WHERE " + map[bool]string{true: "campaign_id", false: "id"}[table == "campaign_drafts"] + "=$1::uuid"
		}
		if err := pool.QueryRow(ctx, query, response.Resource.Id.String()).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	var selector, policy string
	if err := pool.QueryRow(ctx, "SELECT canonical_selector::text,policy_json::text FROM campaign_drafts WHERE campaign_id=$1::uuid", response.Resource.Id.String()).Scan(&selector, &policy); err != nil || !strings.Contains(selector, "fleetforge.selector/v1") || !strings.Contains(policy, "fleetforge.campaign-policy/v1") {
		t.Fatalf("selector=%s policy=%s err=%v", selector, policy, err)
	}
}

func TestControlCampaignPauseRouteCommitsFenceAndReplays(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	artifactID := "00000000-0000-0000-0000-000000000701"
	campaignID := "00000000-0000-0000-0000-000000000702"
	mustExecute(t, pool, ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state) VALUES ('"+artifactID+"','v1','sha256:"+strings.Repeat("b", 64)+"','artifact.bin','VERIFIED')")
	mustExecute(t, pool, ctx, "INSERT INTO campaigns (id,artifact_id,state,revision,command_epoch,policy_version_id) VALUES ('"+campaignID+"','"+artifactID+"','RUNNING_CANARY',3,4,'00000000-0000-0000-0000-000000000703')")
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	service := control.NewCampaignService(persistence.ControlCampaignAdapter{Pool: pool}, func() time.Time { return now })
	verifier := integrationVerifier{principal: access.Principal{Subject: "user:operator", Roles: []access.Role{access.RoleOperator}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}}
	router := controlhttp.NewRouterWithResourcesUploadAndCampaigns(verifier, func() time.Time { return now }, nil, nil, service, nil, "")
	request := httptest.NewRequest(http.MethodPost, "/v1/campaigns/"+campaignID+":pause", nil)
	request.Header.Set("Authorization", "Bearer operator")
	request.Header.Set("Idempotency-Key", "campaign-pause-0001")
	request.Header.Set("If-Match", "\"rev-3\"")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	replay := httptest.NewRequest(http.MethodPost, "/v1/campaigns/"+campaignID+":pause", nil)
	replay.Header.Set("Authorization", "Bearer operator")
	replay.Header.Set("Idempotency-Key", "campaign-pause-0001")
	replay.Header.Set("If-Match", "\"rev-3\"")
	replayRecorder := httptest.NewRecorder()
	router.ServeHTTP(replayRecorder, replay)
	if replayRecorder.Code != http.StatusAccepted || replayRecorder.Body.String() != recorder.Body.String() {
		t.Fatalf("replay status=%d body=%s want=%s", replayRecorder.Code, replayRecorder.Body.String(), recorder.Body.String())
	}
	var state, suspended string
	var revision, epoch int64
	if err := pool.QueryRow(ctx, "SELECT state,revision,command_epoch,suspended_phase FROM campaigns WHERE id=$1::uuid", campaignID).Scan(&state, &revision, &epoch, &suspended); err != nil || state != "PAUSED" || revision != 4 || epoch != 5 || suspended != "RUNNING_CANARY" {
		t.Fatalf("campaign=%s/%d/%d/%s err=%v", state, revision, epoch, suspended, err)
	}
	for _, table := range []string{"audit_events", "decision_inputs", "outbox_events"} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table+" WHERE aggregate_id=$1::uuid", campaignID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
}

func TestControlCampaignStartRouteCommitsCanaryWorkAndReplays(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	artifactID, targetID := "00000000-0000-0000-0000-000000000711", "00000000-0000-0000-0000-000000000712"
	campaignID, targetUpdateID := "00000000-0000-0000-0000-000000000713", "00000000-0000-0000-0000-000000000714"
	campaignTargetID, candidateID := "00000000-0000-0000-0000-000000000715", "00000000-0000-0000-0000-000000000716"
	digest := "sha256:" + strings.Repeat("c", 64)
	mustExecute(t, pool, ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state) VALUES ('"+artifactID+"','v1','"+digest+"','artifact.bin','VERIFIED')")
	mustExecute(t, pool, ctx, "INSERT INTO targets (id,external_identity,endpoint_reference,site_key,rack_key,administrative_state) VALUES ('"+targetID+"','start-target','ref','site-a','rack-a','ACTIVE')")
	mustExecute(t, pool, ctx, "INSERT INTO campaigns (id,artifact_id,state,revision,policy_version_id) VALUES ('"+campaignID+"','"+artifactID+"','VALIDATING',1,'00000000-0000-0000-0000-000000000717')")
	mustExecute(t, pool, ctx, "INSERT INTO preflight_candidates (id,artifact_id,artifact_version_id,selector_version,selector_json,snapshot_hash,state,report) VALUES ('"+candidateID+"','"+artifactID+"','version-1','selector-v1','{}','"+digest+"','PASSED','{}')")
	mustExecute(t, pool, ctx, "INSERT INTO campaign_plan_snapshots (campaign_id,candidate_id,draft_revision,policy_json,effective_plan_json,seed,policy_version,algorithm_version,plan_version,artifact_version_id,artifact_digest,verification_policy_version,target_snapshot_hash,plan_hash) VALUES ('"+campaignID+"','"+candidateID+"',1,'{}','{}','seed','fleetforge.campaign-policy/v1','selection/v1','campaign-plan/v1','version-1','"+digest+"','synthetic-sha256-v1','"+digest+"','"+digest+"')")
	mustExecute(t, pool, ctx, "UPDATE campaigns SET state='READY',frozen_policy_hash='"+digest+"',frozen_target_snapshot_hash='"+digest+"' WHERE id='"+campaignID+"'")
	mustExecute(t, pool, ctx, "INSERT INTO campaign_targets (id,campaign_id,target_id,artifact_id,frozen_target_revision,frozen_policy_hash,cohort) VALUES ('"+campaignTargetID+"','"+campaignID+"','"+targetID+"','"+artifactID+"',0,'policy-hash','CANARY')")
	mustExecute(t, pool, ctx, "INSERT INTO target_updates (id,campaign_target_id,state,expected_firmware_version) VALUES ('"+targetUpdateID+"','"+campaignTargetID+"','PENDING','v1')")
	mustExecute(t, pool, ctx, "INSERT INTO target_enrollments (id,target_id,campaign_id,campaign_target_id) VALUES ('00000000-0000-0000-0000-000000000718','"+targetID+"','"+campaignID+"','"+campaignTargetID+"')")
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	service := control.NewCampaignService(persistence.ControlCampaignAdapter{Pool: pool}, func() time.Time { return now })
	verifier := integrationVerifier{principal: access.Principal{Subject: "user:operator", Roles: []access.Role{access.RoleOperator}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}}
	router := controlhttp.NewRouterWithResourcesUploadAndCampaigns(verifier, func() time.Time { return now }, nil, nil, service, nil, "")
	request := httptest.NewRequest(http.MethodPost, "/v1/campaigns/"+campaignID+":start", nil)
	request.Header.Set("Authorization", "Bearer operator")
	request.Header.Set("Idempotency-Key", "campaign-start-0001")
	request.Header.Set("If-Match", "\"rev-1\"")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	replay := httptest.NewRequest(http.MethodPost, "/v1/campaigns/"+campaignID+":start", nil)
	replay.Header.Set("Authorization", "Bearer operator")
	replay.Header.Set("Idempotency-Key", "campaign-start-0001")
	replay.Header.Set("If-Match", "\"rev-1\"")
	replayRecorder := httptest.NewRecorder()
	router.ServeHTTP(replayRecorder, replay)
	if replayRecorder.Code != http.StatusAccepted || replayRecorder.Body.String() != recorder.Body.String() {
		t.Fatalf("replay status=%d body=%s want=%s", replayRecorder.Code, replayRecorder.Body.String(), recorder.Body.String())
	}
	var state string
	var revision int64
	if err := pool.QueryRow(ctx, "SELECT state,revision FROM campaigns WHERE id=$1::uuid", campaignID).Scan(&state, &revision); err != nil || state != "RUNNING_CANARY" || revision != 2 {
		t.Fatalf("campaign=%s/%d err=%v", state, revision, err)
	}
	var workCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM due_work WHERE subject_id=$1::uuid AND work_type='CANARY_START'", targetUpdateID).Scan(&workCount); err != nil || workCount != 1 {
		t.Fatalf("canary work=%d err=%v", workCount, err)
	}
}
