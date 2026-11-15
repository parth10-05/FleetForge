//go:build postgresintegration

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	app "github.com/parth10-05/fleetforge/internal/application/campaignvalidation"
	"github.com/parth10-05/fleetforge/internal/domain/campaignplan"
	"github.com/parth10-05/fleetforge/internal/domain/value"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestFinalizeCampaignValidationCommitsReadyAndExactLeaseTogether(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	const artifactID = "10000000-0000-4000-8000-000000000001"
	const targetID = "10000000-0000-4000-8000-000000000002"
	const campaignID = "10000000-0000-4000-8000-000000000003"
	const policyVersionID = "10000000-0000-4000-8000-000000000004"
	const commandID = "10000000-0000-4000-8000-000000000005"
	const workID = "10000000-0000-4000-8000-000000000006"
	const leaseToken = "10000000-0000-4000-8000-000000000007"
	const correlationID = "10000000-0000-4000-8000-000000000008"
	const decisionID = "10000000-0000-4000-8000-000000000009"
	const outboxID = "10000000-0000-4000-8000-000000000010"

	seedValidationFinalizationFixture(t, pool, ctx, artifactID, targetID, campaignID, policyVersionID, commandID, workID, leaseToken, correlationID)
	snapshot := mustCandidateSnapshot(t, artifactID, targetID, "rack-a")
	policy := validationFinalizationPolicy()
	parsedTarget, err := value.ParseTargetID(targetID)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := campaignplan.Build(campaignplan.Input{Policy: policy, Seed: "validation-seed", Algorithm: campaignplan.SelectionVersionV1, Candidates: []campaignplan.Candidate{{TargetID: parsedTarget, Revision: 0, Site: "west", Rack: "rack-a", Family: "synthetic"}}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	request := app.FinalizationRequest{
		Work:        app.Work{ID: workID, CommandID: commandID, CampaignID: campaignID, LeaseToken: leaseToken, ExpectedRevision: 1},
		CandidateID: "10000000-0000-4000-8000-000000000011", CanaryWaveID: "10000000-0000-4000-8000-000000000012", LaterWaveID: "10000000-0000-4000-8000-000000000013", DecisionInputID: decisionID, OutboxEventID: outboxID,
		Snapshot: snapshot, Report: []byte(`{"state":"passed","target_count":1}`), Plan: plan, PolicyVersionID: policyVersionID, Now: now,
		Members: []app.PlanMember{{TargetID: targetID, CampaignTargetID: "10000000-0000-4000-8000-000000000014", TargetUpdateID: "10000000-0000-4000-8000-000000000015", EnrollmentID: "10000000-0000-4000-8000-000000000016"}},
	}
	if err := persistence.FinalizeCampaignValidation(ctx, pool, request); err != nil {
		t.Fatal(err)
	}
	var state, workState string
	var revision int64
	if err := pool.QueryRow(ctx, "SELECT state,revision FROM campaigns WHERE id=$1::uuid", campaignID).Scan(&state, &revision); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM due_work WHERE id=$1::uuid", workID).Scan(&workState); err != nil {
		t.Fatal(err)
	}
	if state != "READY" || revision != 2 || workState != "COMPLETED" {
		t.Fatalf("campaign=%s/%d work=%s, want READY/2 and COMPLETED", state, revision, workState)
	}
	for _, table := range []string{"preflight_candidates", "campaign_plan_snapshots", "campaign_targets", "target_enrollments", "decision_inputs", "audit_events", "outbox_events"} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count == 0 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
}

func seedValidationFinalizationFixture(t *testing.T, pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, ctx context.Context, artifactID, targetID, campaignID, policyVersionID, commandID, workID, leaseToken, correlationID string) {
	t.Helper()
	policy, err := validationFinalizationPolicy().CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	mustExecute(t, pool, ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state) VALUES ('"+artifactID+"','v2','sha256:"+strings.Repeat("a", 64)+"','bound','VERIFIED')")
	mustExecute(t, pool, ctx, "INSERT INTO artifact_object_versions (artifact_id,provider,container,object_key,version_id,object_size,sha256_digest,verification_policy_version,declared_target_family) VALUES ('"+artifactID+"','local-versioned-v1','artifacts','artifacts/v1/firmware.bin','version-1',3,'sha256:"+strings.Repeat("a", 64)+"','synthetic-sha256-v1','synthetic')")
	mustExecute(t, pool, ctx, "INSERT INTO targets (id,external_identity,endpoint_reference,site_key,rack_key,administrative_state,target_key,contract_version,credential_provider,credential_reference,transport_policy_version,registration_hash,trust_metadata_complete) VALUES ('"+targetID+"','validation-target','https://simulator.example','west','rack-a','ACTIVE','validation-target','synthetic-redfish-v1','synthetic-test','synthetic-test:validation-target','synthetic-transport-v1','sha256:"+strings.Repeat("a", 64)+"',true)")
	mustExecute(t, pool, ctx, "INSERT INTO target_labels (target_id,key,value) VALUES ('"+targetID+"','family','synthetic'),('"+targetID+"','site','west'),('"+targetID+"','rack','rack-a')")
	mustExecute(t, pool, ctx, "INSERT INTO device_observations (id,target_id,kind,observed_at,normalized_data) VALUES ('10000000-0000-4000-8000-000000000017','"+targetID+"','INVENTORY',now(),'{}')")
	mustExecute(t, pool, ctx, "INSERT INTO target_discovery_heads (target_id,head_revision,configuration_revision,latest_attempt_id,last_good_id) VALUES ('"+targetID+"',0,0,'10000000-0000-4000-8000-000000000017','10000000-0000-4000-8000-000000000017')")
	mustExecute(t, pool, ctx, "INSERT INTO campaigns (id,artifact_id,state,revision,policy_version_id) VALUES ('"+campaignID+"','"+artifactID+"','VALIDATING',1,'"+policyVersionID+"')")
	mustExecute(t, pool, ctx, "INSERT INTO campaign_drafts (campaign_id,artifact_id,canonical_selector,policy_json,selection_seed,selection_algorithm,created_by) VALUES ('"+campaignID+"','"+artifactID+"','{\"version\":\"fleetforge.selector/v1\",\"all\":[{\"key\":\"site\",\"value\":\"west\"}]}'::jsonb,'"+string(policy)+"'::jsonb,'validation-seed','selection/v1','user:operator')")
	mustExecute(t, pool, ctx, "INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,command_id,correlation_id,causation_id,action,reason_code) VALUES ('CAMPAIGN','"+campaignID+"',1,0,'user:operator','"+commandID+"','"+correlationID+"','"+commandID+"','CAMPAIGN_VALIDATION_REQUESTED','campaign.validation.requested')")
	mustExecute(t, pool, ctx, "INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,delivery_state,lease_token,lease_owner,lease_expires_at) VALUES ('"+workID+"','"+commandID+"','CAMPAIGN','"+campaignID+"','CAMPAIGN_VALIDATE',1,now(),1,'CLAIMED','"+leaseToken+"','worker',now()+interval '1 minute')")
}

func validationFinalizationPolicy() campaignplan.Policy {
	return campaignplan.Policy{Version: campaignplan.PolicyVersionV1, Canary: campaignplan.Size{Unit: campaignplan.SizeUnitAbsolute, Value: 1}, Limits: campaignplan.Limits{Global: 1, Site: 1, Rack: 1}, FailureBudget: campaignplan.Fraction{Numerator: 1, Denominator: 1}, ExpectedFirmwareVersion: "v2", RequiredHealthChecks: []string{"boot"}, Maintenance: campaignplan.MaintenanceWindow{Timezone: "UTC", Weekdays: []int{1}, StartMinute: 1, EndMinute: 2}, MaximumDurationSeconds: 60, RetryBudget: 1, ObservationWindowSecond: 30, ObservationEverySecond: 10, ObservationFreshSecond: 10, GovernanceID: "change-1"}
}
