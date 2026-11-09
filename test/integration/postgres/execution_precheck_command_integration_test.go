//go:build postgresintegration

package postgres

import (
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/execution"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestPrecheckCommandUsesFrozenPolicyAndVerifiedObjectIdentity(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	const epoch = int64(7)
	const digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	policy := `{"version":"fleetforge.campaign-policy/v1","canary":{"unit":"absolute","value":1},"limits":{"global":3,"site":2,"rack":1},"failure_budget":{"numerator":0,"denominator":1},"expected_firmware_version":"v1","required_health_checks":["boot"],"maintenance":{"timezone":"UTC","weekdays":[0,1,2,3,4,5,6],"start_minute":0,"end_minute":1439},"maximum_duration_seconds":3600,"retry_budget":1,"observation_window_seconds":60,"observation_every_seconds":10,"observation_freshness_seconds":60,"governance_id":"test-governance","strict_worst_case_capacity":false}`
	mustExecute(t, pool, ctx, "UPDATE campaigns SET state='RUNNING_WAVES',command_epoch=7 WHERE id='"+fixture.campaignID+"'")
	mustExecute(t, pool, ctx, "UPDATE target_updates SET state='PRECHECK',revision=1,command_epoch=7 WHERE id='"+fixture.targetUpdateID+"'")
	mustExecute(t, pool, ctx, "UPDATE targets SET target_key='target-race-key',contract_version='synthetic-redfish-v1',credential_provider='synthetic-test',credential_reference='synthetic-test:target-race',transport_policy_version='transport-v1',registration_hash='sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc',trust_metadata_complete=true WHERE id='"+fixture.targetID+"'")
	mustExecute(t, pool, ctx, "INSERT INTO artifact_object_versions (artifact_id,provider,container,object_key,version_id,object_size,sha256_digest,verification_policy_version,declared_target_family) VALUES ('"+fixture.artifactID+"','local-versioned-v1','fixture','firmware-v1','version-1',1,'"+digest+"','synthetic-sha256-v1','synthetic')")
	mustExecute(t, pool, ctx, "INSERT INTO preflight_candidates (id,artifact_id,artifact_version_id,selector_version,selector_json,snapshot_hash,state,report) VALUES ('"+fixture.candidateID+"','"+fixture.artifactID+"','version-1','selector-v1','{}','sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb','PASSED','{}')")
	mustExecute(t, pool, ctx, "INSERT INTO campaign_plan_snapshots (campaign_id,candidate_id,draft_revision,policy_json,effective_plan_json,seed,policy_version,algorithm_version,plan_version,artifact_version_id,artifact_digest,verification_policy_version,target_snapshot_hash,plan_hash) VALUES ('"+fixture.campaignID+"','"+fixture.candidateID+"',1,'"+policy+"','{}','seed','fleetforge.campaign-policy/v1','deterministic/v1','campaign-plan/v1','version-1','"+digest+"','synthetic-sha256-v1','sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb','sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc')")
	const workID = "00000000-0000-0000-5401-000000000001"
	const commandID = "00000000-0000-0000-5401-000000000002"
	const lease = "00000000-0000-4000-8000-000000000003"
	mustExecute(t, pool, ctx, "INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,expected_command_epoch) VALUES ('"+workID+"','"+commandID+"','TARGET_UPDATE','"+fixture.targetUpdateID+"','PRECHECK',1,now()-interval '1 second',1,7)")
	now := time.Now().UTC()
	adapter := persistence.ExecutionAdapter{Pool: pool, Now: func() time.Time { return now }}
	work, claimed, err := adapter.ClaimPrecheckWork(ctx, "precheck-test", lease)
	if err != nil || !claimed {
		t.Fatalf("ClaimPrecheckWork() work=%+v claimed=%t err=%v", work, claimed, err)
	}
	command, credential, err := adapter.LoadPrecheckCommand(ctx, work)
	if err != nil {
		t.Fatalf("LoadPrecheckCommand() error=%v", err)
	}
	if credential != "synthetic-test:target-race" || command.RequestFingerprint != digest || command.Artifact.ObjectVersion != "version-1" || command.Artifact.SHA256 != digest || command.Snapshot.Revision != 1 || command.Snapshot.CommandEpoch != epoch {
		t.Fatalf("command did not preserve immutable inputs: credential=%q command=%+v", credential, command)
	}
	if command.Envelope.Global.Limit != 3 || command.Envelope.Site.Limit != 2 || command.Envelope.Rack.Limit != 1 || command.Envelope.Site.Key != "site-a" || command.Envelope.Rack.Key != "rack-a" {
		t.Fatalf("capacity=%+v/%+v/%+v, want frozen policy limits and target scopes", command.Envelope.Global, command.Envelope.Site, command.Envelope.Rack)
	}
	if command.AttemptID == "" || command.AdmissionID == "" || command.Envelope.DueWorkID == "" || command.AlreadyCurrentEnvelope.OutboxEventID == "" {
		t.Fatalf("command missing deterministic workflow identities: %+v", command)
	}
	command.PrecheckEvidence = execution.PrecheckEvidence{CurrentVersion: "1.0", Capable: true, Healthy: true, Compatible: true, EvidenceFingerprint: digest, ObservedAt: now}
	if _, err := adapter.AdmitSubmission(ctx, command); err != nil {
		t.Fatalf("AdmitSubmission() error=%v", err)
	}
	var submitCommandID string
	if err := pool.QueryRow(ctx, "SELECT command_id::text FROM due_work WHERE work_type='SUBMIT'").Scan(&submitCommandID); err != nil || submitCommandID == command.Work.CommandID {
		t.Fatalf("submit command=%q precheck command=%q err=%v, want distinct command identities", submitCommandID, command.Work.CommandID, err)
	}
}
