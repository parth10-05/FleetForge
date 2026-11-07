package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/campaign"
	"github.com/parth10-05/fleetforge/internal/domain/policy"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

// FinalizeEligibleCanaryOnce invokes the existing fenced canary reducer only
// after every canary target is terminal. It reads no device state and does no
// external I/O; FinalizeCanary owns the serializable transition and evidence.
func FinalizeEligibleCanaryOnce(ctx context.Context, pool *pgxpool.Pool, now time.Time) (bool, error) {
	if pool == nil || now.IsZero() {
		return false, fmt.Errorf("invalid canary finalization worker request")
	}
	var campaignID string
	var revision, epoch int64
	err := pool.QueryRow(ctx, `SELECT c.id::text,c.revision,c.command_epoch
FROM campaigns c
WHERE c.state='RUNNING_CANARY'
  AND NOT EXISTS (
    SELECT 1 FROM campaign_targets ct
    JOIN target_updates tu ON tu.campaign_target_id=ct.id
    WHERE ct.campaign_id=c.id AND ct.cohort='CANARY'
      AND tu.state NOT IN ('SUCCEEDED','FAILED_TERMINAL','SKIPPED','CANCELLED','NEEDS_ATTENTION')
  )
ORDER BY c.updated_at,c.id LIMIT 1`).Scan(&campaignID, &revision, &epoch)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("select terminal canary: %w", err)
	}
	rows, err := pool.Query(ctx, `SELECT tu.id::text
FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id
WHERE ct.campaign_id=$1::uuid AND ct.cohort='LATER_WAVE'
ORDER BY ct.id,tu.id`, campaignID)
	if err != nil {
		return false, fmt.Errorf("load later-wave updates: %w", err)
	}
	defer rows.Close()
	later := make([]CanaryWorkIntent, 0)
	for rows.Next() {
		var targetUpdateID string
		if err := rows.Scan(&targetUpdateID); err != nil {
			return false, fmt.Errorf("scan later-wave update: %w", err)
		}
		base := "fleetforge.canary-finalization/v1:" + campaignID + ":" + targetUpdateID
		later = append(later, CanaryWorkIntent{DueWorkID: deterministicExecutionUUID(base + ":work"), CommandID: deterministicExecutionUUID(base + ":command"), TargetUpdateID: targetUpdateID})
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("read later-wave updates: %w", err)
	}
	base := "fleetforge.canary-finalization/v1:" + campaignID
	err = FinalizeCanary(ctx, pool, FinalizeCanaryRequest{
		CampaignID: campaignID, TerminalCommandID: deterministicExecutionUUID(base + ":terminal"), ResolutionCommandID: deterministicExecutionUUID(base + ":resolution"),
		ExpectedRevision: revision, ExpectedEpoch: epoch, LogicalCut: now.UTC(), Now: now.UTC(), ActorID: "system:campaign-finalizer", CorrelationID: deterministicExecutionUUID(base + ":correlation"), CausationID: deterministicExecutionUUID(base + ":causation"),
		AuditAction: "CANARY_APPROVED", OutboxEventID: deterministicExecutionUUID(base + ":outbox"), DecisionInputID: deterministicExecutionUUID(base + ":decision"), ReducerVersion: "campaign-reducer/v1", LaterIntents: later,
	})
	if errors.Is(err, ErrCanaryEvaluationPending) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CompleteEligibleCampaignOnce closes a fully successful later-wave campaign.
// It intentionally requires every target update to be SUCCEEDED; less certain
// outcomes remain visible for the existing pause/attention flows to resolve.
func CompleteEligibleCampaignOnce(ctx context.Context, pool *pgxpool.Pool, now time.Time) (bool, error) {
	if pool == nil || now.IsZero() {
		return false, fmt.Errorf("invalid campaign completion worker request")
	}
	var campaignID string
	var revision, epoch int64
	err := pool.QueryRow(ctx, `SELECT c.id::text,c.revision,c.command_epoch
FROM campaigns c
WHERE c.state='RUNNING_WAVES'
  AND EXISTS (SELECT 1 FROM campaign_targets ct WHERE ct.campaign_id=c.id)
  AND NOT EXISTS (
    SELECT 1 FROM campaign_targets ct
    JOIN target_updates tu ON tu.campaign_target_id=ct.id
    WHERE ct.campaign_id=c.id AND tu.state <> 'SUCCEEDED'
  )
ORDER BY c.updated_at,c.id LIMIT 1`).Scan(&campaignID, &revision, &epoch)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("select completed campaign: %w", err)
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, fmt.Errorf("begin campaign completion: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var state, policyVersion string
	if err := tx.QueryRow(ctx, "SELECT state,policy_version_id::text FROM campaigns WHERE id=$1::uuid AND revision=$2 AND command_epoch=$3 FOR UPDATE", campaignID, revision, epoch).Scan(&state, &policyVersion); err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("lock campaign completion: %w", err)
	}
	rev, _ := value.NewRevision(revision)
	ep, _ := value.NewCommandEpoch(epoch)
	commandID, err := value.ParseCommandID(deterministicExecutionUUID("fleetforge.campaign-completion/v1:" + campaignID + ":command"))
	if err != nil {
		return false, err
	}
	decision := campaign.Reduce(campaign.Aggregate{State: campaign.State(state), Revision: rev, CommandEpoch: ep}, campaign.CommandInput{ID: commandID, Command: campaign.CommandComplete, ExpectedRevision: rev, ExpectedEpoch: ep, Authorization: policy.NewCampaignAuthorization(policy.CampaignGuardAllTargetsTerminal, policy.CampaignGuardFinalPolicyPassed)})
	if decision.Class != campaign.DecisionAllowed {
		return false, nil
	}
	base := "fleetforge.campaign-completion/v1:" + campaignID
	newRevision := int64(decision.Aggregate.Revision.Uint64())
	if _, err := tx.Exec(ctx, `INSERT INTO decision_inputs (id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code)
VALUES (md5($1)::uuid,'CAMPAIGN',$2::uuid,$3,0,$4,'campaign-reducer/v1',$5,'{"all_targets":"succeeded"}'::jsonb,'COMPLETED','campaign.wave_advanced')`, base+":decision", campaignID, newRevision, now.UTC(), policyVersion); err != nil {
		return false, fmt.Errorf("record campaign completion decision: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,command_id,correlation_id,causation_id,action,reason_code)
VALUES ('CAMPAIGN',$1::uuid,$2,0,'system:campaign-finalizer',$3::uuid,$3::uuid,$3::uuid,'CAMPAIGN_COMPLETED','campaign.wave_advanced')`, campaignID, newRevision, commandID.String()); err != nil {
		return false, fmt.Errorf("record campaign completion audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload)
VALUES (md5($1)::uuid,'CAMPAIGN',$2::uuid,$3,'campaign.completed',1,'{"state":"COMPLETED"}'::jsonb)`, base+":outbox", campaignID, newRevision); err != nil {
		return false, fmt.Errorf("record campaign completion outbox: %w", err)
	}
	tag, err := tx.Exec(ctx, "UPDATE campaigns SET state='COMPLETED',revision=$3,updated_at=$4 WHERE id=$1::uuid AND revision=$2 AND command_epoch=$5", campaignID, revision, newRevision, now.UTC(), epoch)
	if err != nil {
		return false, fmt.Errorf("complete campaign: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit campaign completion: %w", err)
	}
	return true, nil
}
