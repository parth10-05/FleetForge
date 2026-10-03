// campaign_evaluation.go loads the complete, frozen fact set for the P03
// canary evaluator. It deliberately performs no worker or device action.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/campaignplan"
)

// LoadCanaryEvaluationInput reads the immutable policy plus the exact canary
// cohort facts required by the pure evaluator at a caller-supplied logical
// cut. FinalizeCanary uses the transaction-scoped form below so its decision
// and reducer transition share one serializable transaction.
func LoadCanaryEvaluationInput(ctx context.Context, pool *pgxpool.Pool, campaignID string, logicalCut time.Time) (campaignplan.EvaluationInput, error) {
	if _, err := uuidParameter(campaignID); err != nil || logicalCut.IsZero() {
		return campaignplan.EvaluationInput{}, fmt.Errorf("invalid canary evaluation identity or logical cut")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return campaignplan.EvaluationInput{}, fmt.Errorf("begin canary evaluation read: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	input, err := loadCanaryEvaluationInputTx(ctx, tx, campaignID, logicalCut)
	if err != nil {
		return campaignplan.EvaluationInput{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return campaignplan.EvaluationInput{}, fmt.Errorf("commit canary evaluation read: %w", err)
	}
	return input, nil
}

// loadCanaryEvaluationInputTx reads the frozen evidence within a caller-owned
// transaction. It intentionally shares the finalizer's serializable snapshot:
// a decision can therefore never authorize a different set of facts than the
// state transition it records.
func loadCanaryEvaluationInputTx(ctx context.Context, tx pgx.Tx, campaignID string, logicalCut time.Time) (campaignplan.EvaluationInput, error) {
	var policyJSON []byte
	var startedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT p.policy_json,c.updated_at FROM campaigns c JOIN campaign_plan_snapshots p ON p.campaign_id=c.id WHERE c.id=$1::uuid AND c.state IN ('RUNNING_CANARY','EVALUATING_CANARY')`, campaignID).Scan(&policyJSON, &startedAt); err != nil {
		return campaignplan.EvaluationInput{}, fmt.Errorf("load frozen campaign policy: %w", err)
	}
	var policy campaignplan.Policy
	if err := json.Unmarshal(policyJSON, &policy); err != nil {
		return campaignplan.EvaluationInput{}, fmt.Errorf("decode frozen campaign policy: %w", err)
	}
	input := campaignplan.EvaluationInput{Policy: policy, StartedAt: startedAt.UTC(), LogicalCut: logicalCut.UTC()}
	rows, err := tx.Query(ctx, `SELECT ct.target_id::text,tu.state,(e.id IS NOT NULL AND e.state='ACTIVE'),EXISTS(SELECT 1 FROM operation_reservations r WHERE r.target_id=ct.target_id AND r.state IN ('ACTIVE','QUARANTINED'))
FROM campaign_targets ct JOIN target_updates tu ON tu.campaign_target_id=ct.id LEFT JOIN target_enrollments e ON e.campaign_target_id=ct.id
WHERE ct.campaign_id=$1::uuid AND ct.cohort='CANARY' ORDER BY ct.id`, campaignID)
	if err != nil {
		return campaignplan.EvaluationInput{}, fmt.Errorf("load canary outcomes: %w", err)
	}
	for rows.Next() {
		var outcome campaignplan.TargetOutcome
		if err := rows.Scan(&outcome.TargetID, &outcome.UpdateState, &outcome.ActiveEnrollment, &outcome.UnresolvedReservation); err != nil {
			rows.Close()
			return campaignplan.EvaluationInput{}, fmt.Errorf("scan canary outcome: %w", err)
		}
		input.Outcomes = append(input.Outcomes, outcome)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return campaignplan.EvaluationInput{}, fmt.Errorf("read canary outcomes: %w", err)
	}
	rows.Close()
	health, err := tx.Query(ctx, `SELECT ct.target_id::text,h.check_name,h.scheduled_at,h.observed_at,h.passed FROM campaign_targets ct JOIN campaign_health_evidence h ON h.campaign_target_id=ct.id WHERE ct.campaign_id=$1::uuid AND ct.cohort='CANARY' AND h.scheduled_at <= $2 ORDER BY ct.id,h.check_name,h.scheduled_at DESC,h.observed_at DESC`, campaignID, logicalCut)
	if err != nil {
		return campaignplan.EvaluationInput{}, fmt.Errorf("load scheduled canary health: %w", err)
	}
	seen := map[string]struct{}{}
	for health.Next() {
		var sample campaignplan.HealthSample
		if err := health.Scan(&sample.TargetID, &sample.Check, &sample.ScheduledAt, &sample.ObservedAt, &sample.Passed); err != nil {
			health.Close()
			return campaignplan.EvaluationInput{}, fmt.Errorf("scan scheduled canary health: %w", err)
		}
		key := sample.TargetID + "\x00" + sample.Check
		if _, exists := seen[key]; !exists {
			input.Health = append(input.Health, sample)
			seen[key] = struct{}{}
		}
	}
	if err := health.Err(); err != nil {
		health.Close()
		return campaignplan.EvaluationInput{}, fmt.Errorf("read scheduled canary health: %w", err)
	}
	health.Close()
	return input, nil
}
