// unit_of_work.go — short transaction wrapper for atomic aggregate mutation.
//
// ApplyAggregateMutation executes a single short transaction that atomically:
//  1. Updates the aggregate row with an expected-revision guard.
//  2. Appends an audit event.
//  3. Inserts an outbox event envelope.
//  4. Schedules a due-work intent (optional).
//
// No network client, storage API, queue call, or log exporter is invoked
// inside any transaction callback (AGENTS.md safety rule).
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrRevisionConflict is returned when the expected-revision predicate in a
// save query matches zero rows, indicating a concurrent writer won.
var ErrRevisionConflict = errors.New("aggregate revision conflict")

// AggregateMutation describes everything a single decision commits atomically.
// All fields are validated by the caller before passing to ApplyAggregateMutation.
type AggregateMutation struct {
	// Aggregate identity and revision guard.
	AggregateType    string // "CAMPAIGN" or "TARGET_UPDATE"
	AggregateID      string // UUID string
	ExpectedRevision int64  // current revision; mutation sets revision = current + 1
	NewState         string // the validated next state token

	// Campaign-specific (only used when AggregateType == "CAMPAIGN").
	CommandEpoch             int64
	SuspendedPhase           *string // nullable
	PolicyVersionID          string
	FrozenPolicyHash         *string // nullable
	FrozenTargetSnapshotHash *string // nullable

	// TargetUpdate-specific (only used when AggregateType == "TARGET_UPDATE").
	// CommandEpoch is also used for target updates.
	ResumeStep *string // nullable

	// Audit event fields (required).
	ActorID       string
	CommandID     *string // nullable — background tasks may omit
	CorrelationID string
	CausationID   string
	Action        string
	ReasonCode    string
	BeforeHash    *string
	AfterHash     *string
	EventSequence int64

	// Outbox event fields (required).
	OutboxEventID       string
	OutboxEventType     string
	OutboxSchemaVersion int
	OutboxPayload       json.RawMessage

	// Decision input fields are the durable replay record for the validated
	// Phase 02 reducer decision that produced this mutation.
	DecisionInputID       string
	DecisionSequence      int64
	DecisionLogicalTime   time.Time
	ReducerVersion        string
	DecisionPolicyVersion string
	NormalizedFacts       json.RawMessage
	DecisionResult        string

	// Optional due-work scheduling. If DueWorkID is empty no due-work row is written.
	DueWorkID            string
	DueWorkCommandID     string
	DueWorkSubjectType   string
	DueWorkSubjectID     string
	DueWorkWorkType      string
	DueWorkSchemaVersion int
	DueWorkNotBefore     time.Time
	DueWorkExpRevision   int64
	DueWorkExpEpoch      *int64
}

// ApplyAggregateMutation opens a short serializable transaction and writes all
// fields from the mutation atomically. It returns ErrRevisionConflict when the
// aggregate row was already changed by a concurrent writer.
//
// Callers must not hold a database lock or network connection when calling this
// function. The transaction is committed or rolled back before returning.
func ApplyAggregateMutation(ctx context.Context, pool *pgxpool.Pool, m AggregateMutation) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin aggregate mutation transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // intentional best-effort rollback

	if err := applyAggregateMutationInTx(ctx, tx, m); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: serialization during aggregate commit", ErrRevisionConflict)
		}
		return fmt.Errorf("commit aggregate mutation: %w", err)
	}
	return nil
}

// applyAggregateMutationInTx contains the durable aggregate writes shared by
// normal mutations and the idempotent completion transaction. Keeping this
// sequence inside the caller's transaction is what prevents a completed
// idempotency record from referring to a mutation that did not commit.
func applyAggregateMutationInTx(ctx context.Context, tx pgx.Tx, m AggregateMutation) error {
	if err := applyAggregateRevisionUpdate(ctx, tx, m); err != nil {
		return err
	}
	if err := appendAuditEvent(ctx, tx, m); err != nil {
		return err
	}
	if err := insertDecisionInput(ctx, tx, m); err != nil {
		return err
	}
	if err := insertOutboxEvent(ctx, tx, m); err != nil {
		return err
	}
	if m.DueWorkID != "" {
		if err := insertDueWork(ctx, tx, m); err != nil {
			return err
		}
	}
	return nil
}

func insertDecisionInput(ctx context.Context, tx pgx.Tx, m AggregateMutation) error {
	if m.DecisionInputID == "" || m.DecisionLogicalTime.IsZero() || m.ReducerVersion == "" ||
		m.DecisionPolicyVersion == "" || len(m.NormalizedFacts) == 0 || m.DecisionResult == "" {
		return fmt.Errorf("decision input is required for aggregate mutation")
	}
	_, err := tx.Exec(ctx,
		"INSERT INTO decision_inputs (id, aggregate_type, aggregate_id, aggregate_revision, decision_sequence, "+
			"logical_time, reducer_version, policy_version, normalized_facts, result, reason_code) "+
			"VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11)",
		m.DecisionInputID, m.AggregateType, m.AggregateID, m.ExpectedRevision+1, m.DecisionSequence,
		m.DecisionLogicalTime, m.ReducerVersion, m.DecisionPolicyVersion, m.NormalizedFacts,
		m.DecisionResult, m.ReasonCode,
	)
	if err != nil {
		return fmt.Errorf("insert decision input for %s/%s: %w", m.AggregateType, m.AggregateID, err)
	}
	return nil
}

func applyAggregateRevisionUpdate(ctx context.Context, tx pgx.Tx, m AggregateMutation) error {
	var tag pgconn.CommandTag
	var err error
	switch m.AggregateType {
	case "CAMPAIGN":
		tag, err = tx.Exec(ctx,
			// Q14: save campaign with expected-revision guard.
			"UPDATE campaigns SET state=$3, revision=revision+1, command_epoch=$4, suspended_phase=$5, "+
				"policy_version_id=$6::uuid, frozen_policy_hash=$7, frozen_target_snapshot_hash=$8, updated_at=now() "+
				"WHERE id=$1::uuid AND revision=$2",
			m.AggregateID, m.ExpectedRevision,
			m.NewState, m.CommandEpoch, m.SuspendedPhase,
			m.PolicyVersionID, m.FrozenPolicyHash, m.FrozenTargetSnapshotHash,
		)
	case "TARGET_UPDATE":
		tag, err = tx.Exec(ctx,
			// Q15: save target update with expected-revision guard.
			"UPDATE target_updates SET state=$3, revision=revision+1, command_epoch=$4, resume_step=$5, updated_at=now() "+
				"WHERE id=$1::uuid AND revision=$2",
			m.AggregateID, m.ExpectedRevision,
			m.NewState, m.CommandEpoch, m.ResumeStep,
		)
	default:
		return fmt.Errorf("%w: aggregate_type %q", ErrCorruptPersistedValue, m.AggregateType)
	}
	if err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: concurrent aggregate update", ErrRevisionConflict)
		}
		return fmt.Errorf("update aggregate %s/%s: %w", m.AggregateType, m.AggregateID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s/%s at revision %d", ErrRevisionConflict, m.AggregateType, m.AggregateID, m.ExpectedRevision)
	}
	return nil
}

func appendAuditEvent(ctx context.Context, tx pgx.Tx, m AggregateMutation) error {
	// Q16: insert audit event (append-only trigger enforced by database).
	_, err := tx.Exec(ctx,
		"INSERT INTO audit_events (aggregate_type, aggregate_id, aggregate_revision, event_sequence, "+
			"actor_id, command_id, correlation_id, causation_id, action, reason_code, before_hash, after_hash) "+
			"VALUES ($1, $2::uuid, $3, $4, $5, $6::uuid, $7::uuid, $8::uuid, $9, $10, $11, $12)",
		m.AggregateType, m.AggregateID, m.ExpectedRevision+1, m.EventSequence,
		m.ActorID, m.CommandID, m.CorrelationID, m.CausationID, m.Action, m.ReasonCode, m.BeforeHash, m.AfterHash,
	)
	if err != nil {
		return fmt.Errorf("append audit event for %s/%s: %w", m.AggregateType, m.AggregateID, err)
	}
	return nil
}

func insertOutboxEvent(ctx context.Context, tx pgx.Tx, m AggregateMutation) error {
	// Q17: insert outbox event envelope.
	_, err := tx.Exec(ctx,
		"INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, aggregate_revision, "+
			"event_type, schema_version, payload) "+
			"VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7)",
		m.OutboxEventID, m.AggregateType, m.AggregateID, m.ExpectedRevision+1,
		m.OutboxEventType, m.OutboxSchemaVersion, m.OutboxPayload,
	)
	if err != nil {
		return fmt.Errorf("insert outbox event for %s/%s: %w", m.AggregateType, m.AggregateID, err)
	}
	return nil
}

func insertDueWork(ctx context.Context, tx pgx.Tx, m AggregateMutation) error {
	// Q18: schedule due-work intent.
	_, err := tx.Exec(ctx,
		"INSERT INTO due_work (id, command_id, subject_type, subject_id, work_type, schema_version, "+
			"not_before, expected_revision, expected_command_epoch) "+
			"VALUES ($1::uuid, $2::uuid, $3, $4::uuid, $5, $6, $7, $8, $9)",
		m.DueWorkID, m.DueWorkCommandID, m.DueWorkSubjectType, m.DueWorkSubjectID,
		m.DueWorkWorkType, m.DueWorkSchemaVersion, m.DueWorkNotBefore,
		m.DueWorkExpRevision, m.DueWorkExpEpoch,
	)
	if err != nil {
		return fmt.Errorf("insert due work for %s/%s: %w", m.AggregateType, m.AggregateID, err)
	}
	return nil
}

// isSerialization returns true for PostgreSQL serialization failure (40001)
// and deadlock (40P01) error codes. These are the only codes eligible for a
// bounded outer retry; callers must implement the retry budget themselves.
func isSerialization(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40001" || pgErr.Code == "40P01"
	}
	return false
}
