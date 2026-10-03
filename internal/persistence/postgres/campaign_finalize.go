package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/campaign"
	"github.com/parth10-05/fleetforge/internal/domain/campaignplan"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

// FinalizeCanaryRequest identifies the immutable finalization inputs. The
// finalizer derives the evaluation from persisted facts in its transaction;
// callers cannot supply a release decision.
type FinalizeCanaryRequest struct {
	CampaignID, TerminalCommandID, ResolutionCommandID string
	ExpectedRevision, ExpectedEpoch                    int64
	LogicalCut, Now                                    time.Time
	ActorID, CorrelationID, CausationID                string
	AuditAction, OutboxEventID, DecisionInputID        string
	ReducerVersion                                     string
	LaterIntents                                       []CanaryWorkIntent
}

// FinalizeCanary records terminal/PASSED state for the canary and advances the
// campaign through the reducer. It is intentionally narrow: P04 owns creation
// and execution of later target-update work.
func FinalizeCanary(ctx context.Context, pool *pgxpool.Pool, r FinalizeCanaryRequest) error {
	if r.ExpectedRevision < 0 || r.ExpectedEpoch < 0 || r.LogicalCut.IsZero() || r.Now.IsZero() || r.LogicalCut.After(r.Now) || r.ActorID == "" || r.AuditAction == "" || r.ReducerVersion == "" {
		return fmt.Errorf("invalid canary finalization")
	}
	for _, raw := range []string{r.CampaignID, r.TerminalCommandID, r.ResolutionCommandID, r.CorrelationID, r.CausationID, r.OutboxEventID, r.DecisionInputID} {
		if _, err := uuidParameter(raw); err != nil {
			return fmt.Errorf("invalid canary finalization identity: %w", err)
		}
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin canary finalization: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var state, policyVersion, planHash string
	var revision, epoch int64
	if err := tx.QueryRow(ctx, "SELECT c.state,c.revision,c.command_epoch,c.policy_version_id::text,p.plan_hash FROM campaigns c JOIN campaign_plan_snapshots p ON p.campaign_id=c.id WHERE c.id=$1::uuid FOR UPDATE", r.CampaignID).Scan(&state, &revision, &epoch, &policyVersion, &planHash); err != nil {
		return fmt.Errorf("lock campaign finalization: %w", err)
	}
	if state != "RUNNING_CANARY" || revision != r.ExpectedRevision || epoch != r.ExpectedEpoch {
		alreadyRecorded, recordedErr := finalizationAlreadyRecorded(ctx, tx, r.CampaignID, r.OutboxEventID)
		if recordedErr != nil {
			return recordedErr
		}
		if alreadyRecorded {
			return nil
		}
		return ErrCampaignStartConflict
	}
	evaluation, err := loadCanaryEvaluationInputTx(ctx, tx, r.CampaignID, r.LogicalCut)
	if err != nil {
		return fmt.Errorf("load canary finalization evidence: %w", err)
	}
	decision, err := campaignplan.EvaluateCanary(evaluation)
	if err != nil {
		return fmt.Errorf("evaluate canary finalization evidence: %w", err)
	}
	if decision.State == campaignplan.EvaluationPending {
		return fmt.Errorf("canary evaluation is pending: %s", decision.Reason)
	}
	first, err := value.ParseCommandID(r.TerminalCommandID)
	if err != nil {
		return err
	}
	second, err := value.ParseCommandID(r.ResolutionCommandID)
	if err != nil {
		return err
	}
	rev, _ := value.NewRevision(revision)
	ep, _ := value.NewCommandEpoch(epoch)
	release := decision.State == campaignplan.EvaluationPassed
	transitions, ok := campaign.ReduceTerminalCanary(campaign.Aggregate{State: campaign.State(state), Revision: rev, CommandEpoch: ep}, first, second, release)
	if !ok {
		return ErrCampaignStartConflict
	}
	var canaryID, laterID string
	var laterReleased bool
	if err := tx.QueryRow(ctx, "SELECT id::text FROM campaign_waves WHERE campaign_id=$1::uuid AND cohort='CANARY' FOR UPDATE", r.CampaignID).Scan(&canaryID); err != nil {
		return fmt.Errorf("lock canary wave: %w", err)
	}
	if err := tx.QueryRow(ctx, "SELECT id::text,released FROM campaign_waves WHERE campaign_id=$1::uuid AND cohort='LATER_WAVE' FOR UPDATE", r.CampaignID).Scan(&laterID, &laterReleased); err != nil {
		return fmt.Errorf("lock later wave: %w", err)
	}
	if release && laterReleased {
		return ErrCampaignStartConflict
	}
	evaluationState := "FAILED"
	if release {
		evaluationState = "PASSED"
	}
	if _, err := tx.Exec(ctx, "UPDATE campaign_waves SET terminal=true,evaluation=$2,updated_at=$3 WHERE id=$1::uuid AND terminal=false", canaryID, evaluationState, r.Now); err != nil {
		return fmt.Errorf("resolve canary wave: %w", err)
	}
	if release {
		if _, err := tx.Exec(ctx, "UPDATE campaign_waves SET released=true,updated_at=$2 WHERE id=$1::uuid AND released=false", laterID, r.Now); err != nil {
			return fmt.Errorf("release later wave: %w", err)
		}
		updates, err := lockLaterWaveUpdates(ctx, tx, r.CampaignID)
		if err != nil {
			return err
		}
		if len(updates) != len(r.LaterIntents) {
			return fmt.Errorf("%w: later-wave intent count mismatch", ErrCampaignStartConflict)
		}
		seen := make(map[string]struct{}, len(r.LaterIntents))
		for _, intent := range r.LaterIntents {
			for _, raw := range []string{intent.DueWorkID, intent.CommandID, intent.TargetUpdateID} {
				if _, err := uuidParameter(raw); err != nil {
					return fmt.Errorf("invalid later-wave intent identity: %w", err)
				}
			}
			update, ok := updates[intent.TargetUpdateID]
			if !ok {
				return fmt.Errorf("%w: non-later target update intent", ErrCampaignStartConflict)
			}
			if _, duplicate := seen[intent.TargetUpdateID]; duplicate {
				return fmt.Errorf("%w: duplicate later-wave intent", ErrCampaignStartConflict)
			}
			seen[intent.TargetUpdateID] = struct{}{}
			if _, err := tx.Exec(ctx, "INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,expected_command_epoch) VALUES ($1::uuid,$2::uuid,'TARGET_UPDATE',$3::uuid,'LATER_WAVE_START',1,$4,$5,$6)", intent.DueWorkID, intent.CommandID, intent.TargetUpdateID, r.Now, update.revision, update.epoch); err != nil {
				return fmt.Errorf("schedule later-wave intent: %w", err)
			}
		}
	}
	terminalRevision := int64(transitions.Terminal.Aggregate.Revision.Uint64())
	terminalFacts, err := json.Marshal(map[string]any{"plan_hash": planHash, "canary_evaluation": decision.State, "evaluation_reason": decision.Reason.String(), "failed": decision.Failed, "evaluated": decision.Evaluated, "logical_cut": r.LogicalCut.UTC()})
	if err != nil {
		return fmt.Errorf("encode canary terminal facts: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO decision_inputs (id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code) VALUES ($1::uuid,'CAMPAIGN',$2::uuid,$3,0,$4,$5,$6,$7,$8,$9)", r.TerminalCommandID, r.CampaignID, terminalRevision, r.Now, r.ReducerVersion, policyVersion, terminalFacts, transitions.Terminal.Aggregate.State.String(), transitions.Terminal.Reason.String()); err != nil {
		return fmt.Errorf("append canary terminal decision: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,command_id,correlation_id,causation_id,action,reason_code) VALUES ('CAMPAIGN',$1::uuid,$2,0,$3,$4::uuid,$5::uuid,$6::uuid,'CANARY_TERMINAL',$7)", r.CampaignID, terminalRevision, r.ActorID, r.TerminalCommandID, r.CorrelationID, r.CausationID, transitions.Terminal.Reason.String()); err != nil {
		return fmt.Errorf("append canary terminal audit: %w", err)
	}
	newRevision := int64(transitions.Resolved.Aggregate.Revision.Uint64())
	facts, err := json.Marshal(map[string]any{"plan_hash": planHash, "release": release, "canary_evaluation": decision.State, "evaluation_reason": decision.Reason.String(), "failed": decision.Failed, "evaluated": decision.Evaluated, "logical_cut": r.LogicalCut.UTC()})
	if err != nil {
		return fmt.Errorf("encode canary finalization facts: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO decision_inputs (id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code) VALUES ($1::uuid,'CAMPAIGN',$2::uuid,$3,0,$4,$5,$6,$7,$8,$9)", r.DecisionInputID, r.CampaignID, newRevision, r.Now, r.ReducerVersion, policyVersion, facts, transitions.Resolved.Aggregate.State.String(), transitions.Resolved.Reason.String()); err != nil {
		return fmt.Errorf("append canary finalization decision: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,command_id,correlation_id,causation_id,action,reason_code) VALUES ('CAMPAIGN',$1::uuid,$2,0,$3,$4::uuid,$5::uuid,$6::uuid,$7,$8)", r.CampaignID, newRevision, r.ActorID, r.ResolutionCommandID, r.CorrelationID, r.CausationID, r.AuditAction, transitions.Resolved.Reason.String()); err != nil {
		return fmt.Errorf("append canary finalization audit: %w", err)
	}
	payload, err := json.Marshal(map[string]any{"plan_hash": planHash, "release": release, "state": transitions.Resolved.Aggregate.State.String()})
	if err != nil {
		return fmt.Errorf("encode canary finalization outbox: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload) VALUES ($1::uuid,'CAMPAIGN',$2::uuid,$3,'campaign.canary_evaluated',1,$4)", r.OutboxEventID, r.CampaignID, newRevision, payload); err != nil {
		return fmt.Errorf("append canary finalization outbox: %w", err)
	}
	if _, err := tx.Exec(ctx, "UPDATE campaigns SET state=$3,revision=$4,command_epoch=$5,suspended_phase=$6,updated_at=$7 WHERE id=$1::uuid AND revision=$2", r.CampaignID, r.ExpectedRevision, transitions.Resolved.Aggregate.State.String(), int64(transitions.Resolved.Aggregate.Revision.Uint64()), int64(transitions.Resolved.Aggregate.CommandEpoch.Uint64()), nullableState(transitions.Resolved.Aggregate.SuspendedPhase), r.Now); err != nil {
		return fmt.Errorf("resolve campaign finalization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit canary finalization: %w", err)
	}
	return nil
}

func finalizationAlreadyRecorded(ctx context.Context, tx pgx.Tx, campaignID, outboxEventID string) (bool, error) {
	var exists bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM outbox_events WHERE event_id=$1::uuid AND aggregate_type='CAMPAIGN' AND aggregate_id=$2::uuid AND event_type='campaign.canary_evaluated')", outboxEventID, campaignID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check canary finalization idempotency: %w", err)
	}
	return exists, nil
}
func nullableState(s campaign.State) any {
	if s == "" {
		return nil
	}
	return s.String()
}

type laterWaveUpdate struct{ revision, epoch int64 }

func lockLaterWaveUpdates(ctx context.Context, tx pgx.Tx, campaignID string) (map[string]laterWaveUpdate, error) {
	rows, err := tx.Query(ctx, "SELECT tu.id::text,tu.revision,tu.command_epoch FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id WHERE ct.campaign_id=$1::uuid AND ct.cohort='LATER_WAVE' ORDER BY ct.id,tu.id FOR UPDATE OF ct,tu", campaignID)
	if err != nil {
		return nil, fmt.Errorf("lock later-wave target updates: %w", err)
	}
	defer rows.Close()
	updates := map[string]laterWaveUpdate{}
	for rows.Next() {
		var id string
		var update laterWaveUpdate
		if err := rows.Scan(&id, &update.revision, &update.epoch); err != nil {
			return nil, fmt.Errorf("scan later-wave target update: %w", err)
		}
		updates[id] = update
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read later-wave target updates: %w", err)
	}
	return updates, nil
}
