// campaign_start.go persists the bounded, side-effect-free start of a campaign.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/campaign"
	"github.com/parth10-05/fleetforge/internal/domain/policy"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

// ErrCampaignStartConflict means the immutable READY snapshot, enrollment, or
// expected aggregate revision no longer permits a canary start.
var ErrCampaignStartConflict = errors.New("campaign canary start conflicts with current state")

// CanaryWorkIntent supplies durable identities for one canary target-update
// intent. It is deliberately not an execution request: P04 owns worker claim,
// admission, and all firmware-facing behavior.
type CanaryWorkIntent struct {
	DueWorkID      string
	CommandID      string
	TargetUpdateID string
}

// StartCampaignRequest commits the READY -> RUNNING_CANARY decision together
// with exactly the frozen canary cohort's durable due-work intents.
type StartCampaignRequest struct {
	CampaignID       string
	ExpectedRevision int64
	ExpectedEpoch    int64
	CommandID        string
	ActorID          string
	CorrelationID    string
	CausationID      string
	Now              time.Time

	Intents []CanaryWorkIntent

	AuditAction         string
	AuditEventSequence  int64
	OutboxEventID       string
	OutboxSchemaVersion int
	DecisionInputID     string
	DecisionSequence    int64
	ReducerVersion      string
}

// StartCampaign atomically validates the persisted READY campaign and active
// enrollments, then schedules one PENDING CANARY_START intent per canary target.
// It never claims work or reaches a device, queue, storage system, or network.
func StartCampaign(ctx context.Context, pool *pgxpool.Pool, request StartCampaignRequest) error {
	if err := validateStartCampaignRequest(request); err != nil {
		return err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin campaign start: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // transaction is committed below
	if err := startCampaignTx(ctx, tx, request); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: serialization during campaign start", ErrCampaignStartConflict)
		}
		return fmt.Errorf("commit campaign start: %w", err)
	}
	return nil
}

// startCampaignTx preserves the public StartCampaign semantics while allowing
// an API command to complete its idempotency record in the same transaction.
func startCampaignTx(ctx context.Context, tx pgx.Tx, request StartCampaignRequest) error {
	if err := validateStartCampaignRequest(request); err != nil {
		return err
	}

	var state, policyVersion, planHash string
	var revision, epoch int64
	if err := tx.QueryRow(ctx, `SELECT c.state,c.revision,c.command_epoch,c.policy_version_id::text,p.plan_hash
FROM campaigns c JOIN campaign_plan_snapshots p ON p.campaign_id=c.id
WHERE c.id=$1::uuid FOR UPDATE`, request.CampaignID).Scan(&state, &revision, &epoch, &policyVersion, &planHash); err != nil {
		return fmt.Errorf("lock ready campaign: %w", err)
	}
	if state != campaign.StateReady.String() || revision != request.ExpectedRevision || epoch != request.ExpectedEpoch {
		return ErrCampaignStartConflict
	}
	decision, err := startDecision(state, revision, epoch, request)
	if err != nil {
		return err
	}

	canaries, err := lockCanaryTargetUpdates(ctx, tx, request.CampaignID)
	if err != nil {
		return err
	}
	if err := validateCanaryIntents(request.Intents, canaries); err != nil {
		return err
	}
	if err := lockActiveCanaryEnrollments(ctx, tx, request.CampaignID, canaries); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `UPDATE campaigns SET state=$3,revision=revision+1,updated_at=$4
WHERE id=$1::uuid AND revision=$2 AND state='READY'`, request.CampaignID, request.ExpectedRevision, decision.Aggregate.State.String(), request.Now)
	if err != nil {
		return fmt.Errorf("transition campaign to running canary: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrCampaignStartConflict
	}
	newRevision := request.ExpectedRevision + 1
	facts, err := json.Marshal(struct {
		PlanHash    string `json:"plan_hash"`
		CanaryCount int    `json:"canary_count"`
		CommandID   string `json:"command_id"`
	}{planHash, len(canaries), request.CommandID})
	if err != nil {
		return fmt.Errorf("encode campaign start facts: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,command_id,correlation_id,causation_id,action,reason_code)
VALUES ('CAMPAIGN',$1::uuid,$2,$3,$4,$5::uuid,$6::uuid,$7::uuid,$8,$9)`, request.CampaignID, newRevision, request.AuditEventSequence, request.ActorID, request.CommandID, request.CorrelationID, request.CausationID, request.AuditAction, decision.Reason.String()); err != nil {
		return fmt.Errorf("append campaign start audit: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO decision_inputs (id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code)
VALUES ($1::uuid,'CAMPAIGN',$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10)`, request.DecisionInputID, request.CampaignID, newRevision, request.DecisionSequence, request.Now, request.ReducerVersion, policyVersion, facts, decision.Aggregate.State.String(), decision.Reason.String()); err != nil {
		return fmt.Errorf("append campaign start decision: %w", err)
	}
	payload, err := json.Marshal(struct {
		PlanHash    string `json:"plan_hash"`
		CanaryCount int    `json:"canary_count"`
	}{planHash, len(canaries)})
	if err != nil {
		return fmt.Errorf("encode campaign start outbox: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload)
VALUES ($1::uuid,'CAMPAIGN',$2::uuid,$3,'campaign.canary_started',$4,$5)`, request.OutboxEventID, request.CampaignID, newRevision, request.OutboxSchemaVersion, payload); err != nil {
		return fmt.Errorf("append campaign start outbox: %w", err)
	}
	intents := append([]CanaryWorkIntent(nil), request.Intents...)
	sort.Slice(intents, func(i, j int) bool { return intents[i].TargetUpdateID < intents[j].TargetUpdateID })
	for _, intent := range intents {
		update := canaries[intent.TargetUpdateID]
		if _, err := tx.Exec(ctx, `INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,expected_command_epoch)
VALUES ($1::uuid,$2::uuid,'TARGET_UPDATE',$3::uuid,'CANARY_START',1,$4,$5,$6)`, intent.DueWorkID, intent.CommandID, intent.TargetUpdateID, request.Now, update.revision, update.epoch); err != nil {
			return fmt.Errorf("schedule canary due work: %w", err)
		}
	}
	return nil
}

type lockedCanaryUpdate struct {
	campaignTargetID, targetID string
	revision, epoch            int64
}

func lockCanaryTargetUpdates(ctx context.Context, tx pgx.Tx, campaignID string) (map[string]lockedCanaryUpdate, error) {
	rows, err := tx.Query(ctx, `SELECT ct.id::text,ct.target_id::text,tu.id::text,tu.revision,tu.command_epoch
FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id
WHERE ct.campaign_id=$1::uuid AND ct.cohort='CANARY'
ORDER BY ct.id,tu.id FOR UPDATE OF ct,tu`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("lock canary campaign targets: %w", err)
	}
	defer rows.Close()
	result := map[string]lockedCanaryUpdate{}
	for rows.Next() {
		var updateID string
		var value lockedCanaryUpdate
		if err := rows.Scan(&value.campaignTargetID, &value.targetID, &updateID, &value.revision, &value.epoch); err != nil {
			return nil, fmt.Errorf("scan canary campaign target: %w", err)
		}
		result[updateID] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read canary campaign targets: %w", err)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%w: no frozen canary targets", ErrCampaignStartConflict)
	}
	return result, nil
}

func lockActiveCanaryEnrollments(ctx context.Context, tx pgx.Tx, campaignID string, canaries map[string]lockedCanaryUpdate) error {
	rows, err := tx.Query(ctx, `SELECT ct.id::text,e.campaign_id::text,e.state
FROM campaign_targets ct JOIN target_enrollments e ON e.campaign_target_id=ct.id
WHERE ct.campaign_id=$1::uuid AND ct.cohort='CANARY' ORDER BY ct.id,e.id FOR UPDATE OF e`, campaignID)
	if err != nil {
		return fmt.Errorf("lock canary enrollments: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var membership, enrolledCampaign, state string
		if err := rows.Scan(&membership, &enrolledCampaign, &state); err != nil {
			return fmt.Errorf("scan canary enrollment: %w", err)
		}
		if enrolledCampaign != campaignID || state != "ACTIVE" {
			return fmt.Errorf("%w: canary membership %s lacks an active campaign enrollment", ErrCampaignStartConflict, membership)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read canary enrollments: %w", err)
	}
	if count != len(canaries) {
		return fmt.Errorf("%w: canary enrollment count mismatch", ErrCampaignStartConflict)
	}
	return nil
}

func validateCanaryIntents(intents []CanaryWorkIntent, canaries map[string]lockedCanaryUpdate) error {
	if len(intents) != len(canaries) {
		return fmt.Errorf("%w: canary intent count mismatch", ErrCampaignStartConflict)
	}
	seen := make(map[string]struct{}, len(intents))
	for _, intent := range intents {
		for _, raw := range []string{intent.DueWorkID, intent.CommandID, intent.TargetUpdateID} {
			if _, err := uuidParameter(raw); err != nil {
				return fmt.Errorf("invalid canary work identity: %w", err)
			}
		}
		if _, ok := canaries[intent.TargetUpdateID]; !ok {
			return fmt.Errorf("%w: intent is not a canary target update", ErrCampaignStartConflict)
		}
		if _, duplicate := seen[intent.TargetUpdateID]; duplicate {
			return fmt.Errorf("%w: duplicate canary target update intent", ErrCampaignStartConflict)
		}
		seen[intent.TargetUpdateID] = struct{}{}
	}
	return nil
}

func validateStartCampaignRequest(r StartCampaignRequest) error {
	if r.ExpectedRevision < 0 || r.ExpectedEpoch < 0 || r.Now.IsZero() || r.AuditEventSequence < 0 || r.DecisionSequence < 0 || r.OutboxSchemaVersion <= 0 || r.ActorID == "" || r.CorrelationID == "" || r.CausationID == "" || r.AuditAction == "" || r.ReducerVersion == "" {
		return fmt.Errorf("invalid campaign start request")
	}
	for _, raw := range []string{r.CampaignID, r.CommandID, r.CorrelationID, r.CausationID, r.OutboxEventID, r.DecisionInputID} {
		if _, err := uuidParameter(raw); err != nil {
			return fmt.Errorf("invalid campaign start identity: %w", err)
		}
	}
	return nil
}

func startDecision(state string, revision, epoch int64, request StartCampaignRequest) (campaign.Decision, error) {
	campaignID, err := value.ParseCommandID(request.CommandID)
	if err != nil {
		return campaign.Decision{}, fmt.Errorf("parse campaign command: %w", err)
	}
	currentRevision, err := value.NewRevision(revision)
	if err != nil {
		return campaign.Decision{}, err
	}
	currentEpoch, err := value.NewCommandEpoch(epoch)
	if err != nil {
		return campaign.Decision{}, err
	}
	expectedRevision, err := value.NewRevision(request.ExpectedRevision)
	if err != nil {
		return campaign.Decision{}, err
	}
	expectedEpoch, err := value.NewCommandEpoch(request.ExpectedEpoch)
	if err != nil {
		return campaign.Decision{}, err
	}
	decision := campaign.Reduce(campaign.Aggregate{State: campaign.State(state), Revision: currentRevision, CommandEpoch: currentEpoch}, campaign.CommandInput{ID: campaignID, Command: campaign.CommandStartCanary, ExpectedRevision: expectedRevision, ExpectedEpoch: expectedEpoch, Authorization: policy.NewCampaignAuthorization(policy.CampaignGuardAuthorizedStart)})
	if decision.Class != campaign.DecisionAllowed {
		return campaign.Decision{}, fmt.Errorf("%w: reducer rejected canary start as %s", ErrCampaignStartConflict, decision.Class)
	}
	return decision, nil
}
