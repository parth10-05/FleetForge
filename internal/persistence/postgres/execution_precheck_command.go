package postgres

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/parth10-05/fleetforge/internal/application/execution"
	"github.com/parth10-05/fleetforge/internal/domain/campaignplan"
)

// LoadPrecheckCommand loads the immutable, policy-authorized envelope for one
// currently claimed PRECHECK row. It reads no target response and refuses to
// construct an admission from mutable or unverified artifact state.
func (adapter ExecutionAdapter) LoadPrecheckCommand(ctx context.Context, work execution.WorkFence) (execution.AdmissionCommand, string, error) {
	if adapter.Pool == nil || !validExecutionFence(work) {
		return execution.AdmissionCommand{}, "", fmt.Errorf("invalid precheck command fence")
	}
	var campaignTargetID, targetID, campaignID, updateID, artifactID, expectedVersion, site, rack, credential, policyVersion, parentAttemptID, admissionID, workType string
	var epoch, revision, size int64
	var policyJSON []byte
	var provider, container, objectKey, objectVersion, digest string
	err := adapter.Pool.QueryRow(ctx, `SELECT ct.id::text,ct.target_id::text,ct.campaign_id::text,tu.id::text,ct.artifact_id::text,tu.expected_firmware_version,t.site_key,t.rack_key,t.credential_reference,cps.policy_version,cps.policy_json,o.provider,o.container,o.object_key,o.version_id,o.object_size,o.sha256_digest,tu.command_epoch,tu.revision,d.work_type,COALESCE(fa.id::text,''),COALESCE(sa.id::text,'')
FROM due_work d JOIN target_updates tu ON tu.id=d.subject_id JOIN campaign_targets ct ON ct.id=tu.campaign_target_id
JOIN campaigns c ON c.id=ct.campaign_id JOIN targets t ON t.id=ct.target_id JOIN campaign_plan_snapshots cps ON cps.campaign_id=ct.campaign_id
JOIN firmware_artifacts a ON a.id=ct.artifact_id JOIN artifact_object_versions o ON o.artifact_id=a.id
LEFT JOIN submission_admissions sa ON sa.target_update_id=tu.id LEFT JOIN firmware_update_attempts fa ON fa.id=sa.active_attempt_id
WHERE d.id=$1::uuid AND d.command_id=$2::uuid AND d.delivery_generation=$3 AND d.delivery_state='CLAIMED' AND d.lease_token=$4::uuid AND d.expected_revision=$5 AND d.expected_command_epoch IS NOT DISTINCT FROM $6
AND ((d.work_type='PRECHECK' AND tu.state='PRECHECK') OR (d.work_type='RETRY_PRECHECK' AND tu.state='RETRY_WAIT' AND fa.status='PROVEN_ABSENT' AND sa.retry_authorizations > (SELECT COUNT(*) FROM firmware_update_attempts child WHERE child.submission_admission_id=sa.id AND child.parent_attempt_id IS NOT NULL)))
AND c.state IN ('RUNNING_CANARY','RUNNING_WAVES') AND c.command_epoch=tu.command_epoch AND a.state='VERIFIED' AND t.administrative_state='ACTIVE' AND t.trust_metadata_complete
AND t.contract_version='synthetic-redfish-v1' AND t.credential_provider='synthetic-test'`, work.DueWorkID, work.CommandID, work.DeliveryGeneration, work.LeaseToken, work.ExpectedRevision, work.ExpectedCommandEpoch).Scan(&campaignTargetID, &targetID, &campaignID, &updateID, &artifactID, &expectedVersion, &site, &rack, &credential, &policyVersion, &policyJSON, &provider, &container, &objectKey, &objectVersion, &size, &digest, &epoch, &revision, &workType, &parentAttemptID, &admissionID)
	if err == pgx.ErrNoRows {
		return execution.AdmissionCommand{}, "", fmt.Errorf("%w: immutable precheck inputs unavailable", ErrLeaseConflict)
	}
	if err != nil {
		return execution.AdmissionCommand{}, "", fmt.Errorf("load immutable precheck command: %w", err)
	}
	if revision != work.ExpectedRevision || (work.ExpectedCommandEpoch != nil && epoch != *work.ExpectedCommandEpoch) || credential == "" || size <= 0 {
		return execution.AdmissionCommand{}, "", fmt.Errorf("%w: stale or incomplete precheck command", ErrLeaseConflict)
	}
	var policy campaignplan.Policy
	if err := json.Unmarshal(policyJSON, &policy); err != nil || policy.Version != campaignplan.PolicyVersionV1 || policy.Limits.Global == 0 || policy.Limits.Site == 0 || policy.Limits.Rack == 0 || policyVersion == "" {
		return execution.AdmissionCommand{}, "", fmt.Errorf("invalid frozen campaign policy")
	}
	now := time.Now().UTC()
	if adapter.Now != nil {
		now = adapter.Now().UTC()
	}
	base := "fleetforge.execution.precheck/v1:" + work.DueWorkID + ":" + fmt.Sprint(work.DeliveryGeneration)
	expectedState, action, reason := "PRECHECK", "SUBMISSION_ADMITTED", "target.submission.authorized"
	if workType == "RETRY_PRECHECK" {
		expectedState, action, reason = "RETRY_WAIT", "RETRY_SUBMISSION_ADMITTED", "target.retry_submission.authorized"
	}
	if workType == "RETRY_PRECHECK" && (parentAttemptID == "" || admissionID == "") {
		return execution.AdmissionCommand{}, "", fmt.Errorf("%w: retry admission identity unavailable", ErrLeaseConflict)
	}
	command := execution.AdmissionCommand{AttemptID: deterministicExecutionUUID(base + ":attempt"), StepAttemptID: deterministicExecutionUUID(base + ":step"), AdmissionID: deterministicExecutionUUID(base + ":admission"), RequestFingerprint: digest, RetryParentAttemptID: parentAttemptID, Artifact: execution.ObjectIdentity{Provider: provider, Container: container, ObjectKey: objectKey, ObjectVersion: objectVersion, SHA256: digest, Size: size}, Envelope: execution.AdmissionEnvelope{CampaignTargetID: campaignTargetID, ExpectedTargetState: expectedState, NextTargetState: "SUBMITTING", Step: "SUBMISSION", Global: execution.CapacityLimit{Level: "GLOBAL", Key: "GLOBAL", Limit: int64(policy.Limits.Global)}, Site: execution.CapacityLimit{Level: "SITE", Key: site, Limit: int64(policy.Limits.Site)}, Rack: execution.CapacityLimit{Level: "RACK", Key: rack, Limit: int64(policy.Limits.Rack)}, ReservationIDs: [3]string{deterministicExecutionUUID(base + ":reservation:global"), deterministicExecutionUUID(base + ":reservation:site"), deterministicExecutionUUID(base + ":reservation:rack")}, ActorID: "system:precheck", CorrelationID: work.CommandID, CausationID: work.CommandID, Action: action, ReasonCode: reason, OutboxEventID: deterministicExecutionUUID(base + ":outbox:submission"), OutboxEventType: "target.submission_admitted", OutboxSchemaVersion: 1, OutboxPayload: []byte(`{"admitted":true}`), DueWorkID: deterministicExecutionUUID(base + ":work:submit"), DueWorkWorkType: "SUBMIT", DueWorkSchemaVersion: 1, DueWorkNotBefore: now, DecisionInputID: deterministicExecutionUUID(base + ":decision:submission"), DecisionLogicalTime: now, ReducerVersion: "target-update/v1", DecisionPolicyVersion: policyVersion, NormalizedFacts: []byte(`{"precheck":"eligible"}`), DecisionResult: "SUBMITTING"}, AlreadyCurrentEnvelope: execution.CompletionEnvelope{CampaignTargetID: campaignTargetID, ExpectedTargetState: "PRECHECK", NextTargetState: "SUCCEEDED", ActorID: "system:precheck", CorrelationID: work.CommandID, CausationID: work.CommandID, Action: "FIRMWARE_ALREADY_CURRENT", ReasonCode: "target.verification.sufficient", OutboxEventID: deterministicExecutionUUID(base + ":outbox:already-current"), OutboxEventType: "target.firmware_already_current", OutboxSchemaVersion: 1, OutboxPayload: []byte(`{"succeeded":true}`), DecisionInputID: deterministicExecutionUUID(base + ":decision:already-current"), DecisionLogicalTime: now, ReducerVersion: "target-update/v1", DecisionPolicyVersion: policyVersion, NormalizedFacts: []byte(`{"healthy":true,"version_matches":true}`), DecisionResult: "SUCCEEDED"}}
	command.Work = work
	if workType == "RETRY_PRECHECK" {
		command.AdmissionID = admissionID
	}
	command.Snapshot = execution.PrecheckSnapshot{TargetID: targetID, CampaignID: campaignID, TargetUpdateID: updateID, ArtifactID: artifactID, ExpectedVersion: expectedVersion, CommandEpoch: epoch, Revision: revision}
	return command, credential, nil
}

func deterministicExecutionUUID(value string) string {
	sum := md5.Sum([]byte(value))
	raw := hex.EncodeToString(sum[:])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}
