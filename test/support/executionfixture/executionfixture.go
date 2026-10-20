// Package executionfixture seeds the minimal durable execution graph shared by
// PostgreSQL integration and real-process fault tests. It is test-only and
// deliberately contains no simulator or worker behavior.
package executionfixture

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

type Fixture struct {
	TargetID, ArtifactID, CampaignID, CampaignTargetID, TargetUpdateID, CandidateID string
}

func Seed(ctx context.Context, pool *pgxpool.Pool) (Fixture, error) {
	if pool == nil {
		return Fixture{}, fmt.Errorf("fixture requires PostgreSQL pool")
	}
	fixture := Fixture{
		TargetID: "00000000-0000-0000-0000-000000000201", ArtifactID: "00000000-0000-0000-0000-000000000202", CampaignID: "00000000-0000-0000-0000-000000000203",
		CampaignTargetID: "00000000-0000-0000-0000-000000000204", TargetUpdateID: "00000000-0000-0000-0000-000000000205", CandidateID: "00000000-0000-0000-0000-000000000208",
	}
	for _, statement := range []string{
		fmt.Sprintf("INSERT INTO targets (id, external_identity, endpoint_reference, site_key, rack_key, administrative_state) VALUES ('%s', 'target-race', 'ref', 'site-a', 'rack-a', 'ACTIVE')", fixture.TargetID),
		fmt.Sprintf("INSERT INTO firmware_artifacts (id, firmware_version, sha256_digest, object_reference, state) VALUES ('%s', 'v1', 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'ref', 'VERIFIED')", fixture.ArtifactID),
		fmt.Sprintf("INSERT INTO campaigns (id, artifact_id, state, policy_version_id) VALUES ('%s', '%s', 'DRAFT', '00000000-0000-0000-0000-000000000207')", fixture.CampaignID, fixture.ArtifactID),
		fmt.Sprintf("INSERT INTO campaign_targets (id, campaign_id, target_id, artifact_id, frozen_target_revision, frozen_policy_hash) VALUES ('%s', '%s', '%s', '%s', 0, 'policy-hash')", fixture.CampaignTargetID, fixture.CampaignID, fixture.TargetID, fixture.ArtifactID),
		fmt.Sprintf("INSERT INTO target_updates (id, campaign_target_id, state, expected_firmware_version) VALUES ('%s', '%s', 'PENDING', 'v1')", fixture.TargetUpdateID, fixture.CampaignTargetID),
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			return Fixture{}, err
		}
	}
	return fixture, nil
}

func PrepareAdmission(ctx context.Context, pool *pgxpool.Pool, fixture Fixture, epoch int64) error {
	if _, err := pool.Exec(ctx, "UPDATE campaigns SET state='RUNNING_WAVES', command_epoch=$2 WHERE id=$1::uuid", fixture.CampaignID, epoch); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, "UPDATE target_updates SET command_epoch=$2 WHERE id=$1::uuid", fixture.TargetUpdateID, epoch); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, "INSERT INTO target_enrollments (id,target_id,campaign_id,campaign_target_id) VALUES ('00000000-0000-0000-1000-000000000001',$1::uuid,$2::uuid,$3::uuid)", fixture.TargetID, fixture.CampaignID, fixture.CampaignTargetID)
	return err
}

func SeedPolicySnapshot(ctx context.Context, pool *pgxpool.Pool, fixture Fixture) error {
	const digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const snapshotHash = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const planHash = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	if _, err := pool.Exec(ctx, "INSERT INTO artifact_object_versions (artifact_id,provider,container,object_key,version_id,object_size,sha256_digest,verification_policy_version,declared_target_family) VALUES ($1::uuid,'local-versioned-v1','fixture','firmware-v1','version-1',1,$2,'synthetic-sha256-v1','synthetic')", fixture.ArtifactID, digest); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, "INSERT INTO preflight_candidates (id,artifact_id,artifact_version_id,selector_version,selector_json,snapshot_hash,state,report) VALUES ($1::uuid,$2::uuid,'version-1','selector-v1','{}',$3,'PASSED','{}')", fixture.CandidateID, fixture.ArtifactID, snapshotHash); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, "INSERT INTO campaign_plan_snapshots (campaign_id,candidate_id,draft_revision,policy_json,effective_plan_json,seed,policy_version,algorithm_version,plan_version,artifact_version_id,artifact_digest,verification_policy_version,target_snapshot_hash,plan_hash) VALUES ($1::uuid,$2::uuid,1,'{\"observation_freshness_seconds\":60}','{}','seed','fleetforge.campaign-policy/v1','deterministic/v1','campaign-plan/v1','version-1',$3,'synthetic-sha256-v1',$4,$5)", fixture.CampaignID, fixture.CandidateID, digest, snapshotHash, planHash)
	return err
}

func AdmissionRequest(f Fixture, suffix string) persistence.AdmissionRequest {
	base := "00000000-0000-0000-" + suffix + "-0000000000"
	now := time.Now().UTC()
	req := persistence.AdmissionRequest{CampaignID: f.CampaignID, CampaignTargetID: f.CampaignTargetID, TargetID: f.TargetID, ArtifactID: f.ArtifactID, TargetUpdateID: f.TargetUpdateID, ExpectedTargetRevision: 0, CapturedCommandEpoch: 7, ExpectedTargetState: "PENDING", NextTargetState: "SUBMITTING", CommandID: base + "01", Step: "SUBMISSION", AdmissionID: base + "02", StepAttemptID: base + "03", Global: persistence.ScopeCapacity{Level: "GLOBAL", Key: "GLOBAL", Limit: 2}, Site: persistence.ScopeCapacity{Level: "SITE", Key: "site-a", Limit: 2}, Rack: persistence.ScopeCapacity{Level: "RACK", Key: "rack-a", Limit: 2}, ReservationIDs: [3]string{base + "04", base + "05", base + "06"}, ActorID: "system", CorrelationID: base + "01", CausationID: base + "01", Action: "SUBMISSION_ADMITTED", ReasonCode: "target.submission.authorized", OutboxEventID: base + "07", OutboxEventType: "target.submission_admitted", OutboxSchemaVersion: 1, OutboxPayload: json.RawMessage(`{"admitted":true}`), DueWorkID: base + "08", DueWorkWorkType: "SUBMIT", DueWorkSchemaVersion: 1, DueWorkNotBefore: now, DecisionInputID: base + "09", DecisionLogicalTime: now, ReducerVersion: "domain-v1", DecisionPolicyVersion: "policy-v1", NormalizedFacts: json.RawMessage(`{"eligible":true}`), DecisionResult: "SUBMITTING"}
	req.FirmwareAttempt = &persistence.FirmwareAttemptRequest{AttemptID: base + "10", Provider: "local-versioned-v1", Container: "firmware", ObjectKey: "verified/firmware.bin", ObjectVersion: "v1", ObjectSize: 4096, SHA256Digest: digest("a"), ExpectedVersion: "1.2.3", RequestFingerprint: digest("a")}
	req.PrecheckEvidence = json.RawMessage(`{"current_version":"1.0.0","capable":true,"healthy":true,"compatible":true,"evidence_fingerprint":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}`)
	req.PrecheckObservedAt = now
	return req
}

func digest(character string) string {
	return "sha256:" + character + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}
