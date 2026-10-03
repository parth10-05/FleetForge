//go:build postgresintegration

package postgres

import (
	"context"
	"testing"
	"time"

	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestFinalizeCanaryDerivesReleaseAndRecordsBothTransitions(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareReadinessCampaign(t, pool, ctx, fixture)
	if err := persistence.AcquireReadinessEnrollment(ctx, pool, enrollmentRequest(fixture, "00000000-0000-0000-9301-000000000001", "00000000-0000-0000-9301-000000000002")); err != nil {
		t.Fatalf("acquire canary enrollment: %v", err)
	}
	later := seedSecondTargetForReadiness(t, pool, ctx, fixture)
	mustExecute(t, pool, ctx, "INSERT INTO campaign_waves (id,campaign_id,ordinal,cohort) VALUES ('00000000-0000-0000-9301-000000000010','"+fixture.campaignID+"',1,'CANARY'),('00000000-0000-0000-9301-000000000011','"+fixture.campaignID+"',2,'LATER_WAVE')")
	mustExecute(t, pool, ctx, "UPDATE campaign_targets SET wave_id='00000000-0000-0000-9301-000000000010',cohort='CANARY' WHERE id='"+fixture.campaignTargetID+"'")
	mustExecute(t, pool, ctx, "UPDATE campaign_targets SET wave_id='00000000-0000-0000-9301-000000000011',cohort='LATER_WAVE' WHERE id='"+later.campaignTargetID+"'")
	policy := `{"version":"fleetforge.campaign-policy/v1","canary":{"unit":"absolute","value":1},"limits":{"global":1,"site":1,"rack":1},"failure_budget":{"numerator":0,"denominator":1},"expected_firmware_version":"v1","required_health_checks":["boot"],"maintenance":{"timezone":"UTC","weekdays":[0,1,2,3,4,5,6],"start_minute":0,"end_minute":1439},"maximum_duration_seconds":3600,"retry_budget":1,"observation_window_seconds":60,"observation_every_seconds":10,"observation_freshness_seconds":60,"governance_id":"test-governance","strict_worst_case_capacity":false}`
	mustExecute(t, pool, ctx, "INSERT INTO campaign_plan_snapshots (campaign_id,candidate_id,draft_revision,policy_json,effective_plan_json,seed,policy_version,algorithm_version,plan_version,artifact_version_id,artifact_digest,verification_policy_version,target_snapshot_hash,plan_hash) VALUES ('"+fixture.campaignID+"','"+fixture.candidateID+"',1,'"+policy+"','{}','seed','fleetforge.campaign-policy/v1','deterministic/v1','campaign-plan/v1','version-1','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','synthetic-sha256-v1','sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb','sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc')")
	mustExecute(t, pool, ctx, "UPDATE campaigns SET state='RUNNING_CANARY',revision=2,updated_at=now()-interval '1 minute' WHERE id='"+fixture.campaignID+"'")
	mustExecute(t, pool, ctx, "UPDATE target_updates SET state='SUCCEEDED' WHERE id='"+fixture.targetUpdateID+"'")
	cut := time.Now().UTC()
	if _, err := pool.Exec(ctx, "INSERT INTO campaign_health_evidence (id,campaign_target_id,check_name,scheduled_at,observed_at,passed) VALUES ('00000000-0000-0000-9301-000000000012',$1,'boot',$2,$2,true)", fixture.campaignTargetID, cut.Add(-time.Second)); err != nil {
		t.Fatalf("insert scheduled health evidence: %v", err)
	}
	req := persistence.FinalizeCanaryRequest{
		CampaignID: fixture.campaignID, TerminalCommandID: "00000000-0000-0000-9301-000000000020", ResolutionCommandID: "00000000-0000-0000-9301-000000000021",
		ExpectedRevision: 2, ExpectedEpoch: 0, LogicalCut: cut, Now: cut, ActorID: "test-actor", CorrelationID: "00000000-0000-0000-9301-000000000022", CausationID: "00000000-0000-0000-9301-000000000023",
		AuditAction: "CANARY_APPROVED", OutboxEventID: "00000000-0000-0000-9301-000000000024", DecisionInputID: "00000000-0000-0000-9301-000000000025", ReducerVersion: "campaign-reducer/v1",
		LaterIntents: []persistence.CanaryWorkIntent{{DueWorkID: "00000000-0000-0000-9301-000000000026", CommandID: "00000000-0000-0000-9301-000000000027", TargetUpdateID: later.targetUpdateID}},
	}
	if err := persistence.FinalizeCanary(context.Background(), pool, req); err != nil {
		t.Fatalf("finalize canary: %v", err)
	}
	var state string
	var revision int64
	if err := pool.QueryRow(ctx, "SELECT state,revision FROM campaigns WHERE id=$1::uuid", fixture.campaignID).Scan(&state, &revision); err != nil {
		t.Fatal(err)
	}
	if state != "RUNNING_WAVES" || revision != 4 {
		t.Fatalf("campaign finalization = %s/%d, want RUNNING_WAVES/4", state, revision)
	}
	var terminalAudits, decisions, dueWork int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE aggregate_id=$1::uuid AND action='CANARY_TERMINAL'", fixture.campaignID).Scan(&terminalAudits); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM decision_inputs WHERE aggregate_id=$1::uuid AND aggregate_revision IN (3,4)", fixture.campaignID).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM due_work WHERE work_type='LATER_WAVE_START'").Scan(&dueWork); err != nil {
		t.Fatal(err)
	}
	if terminalAudits != 1 || decisions != 2 || dueWork != 1 {
		t.Fatalf("terminal evidence/audit=%d decisions=%d later due work=%d, want 1/2/1", terminalAudits, decisions, dueWork)
	}
	if err := persistence.FinalizeCanary(context.Background(), pool, req); err != nil {
		t.Fatalf("retry finalization: %v", err)
	}
}
