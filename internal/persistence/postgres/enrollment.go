// enrollment.go — readiness enrollment transaction.
//
// AcquireReadinessEnrollment acquires an enrollment claim for every frozen
// target in a single ordered transaction and then transitions the campaign to
// READY. The lock order follows ADR-021:
//
//  1. Lock the campaign row with SELECT … FOR UPDATE.
//  2. For each target (ordered by ID to prevent deadlock): INSERT enrollment.
//  3. UPDATE the campaign state to READY with an expected-revision guard.
//  4. Append an audit event.
//  5. Insert an outbox event.
//
// If any target's enrollment is already owned by another active campaign or
// its operation reservation is in QUARANTINED state, the entire transaction
// rolls back and ReleaseBlockedBy is returned describing the first conflict.
//
// No network client, storage API, queue call, or log exporter is invoked
// inside the transaction callback.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrEnrollmentBlocked is returned when at least one target already has an
// active enrollment or an unresolved quarantine that prevents readiness.
var ErrEnrollmentBlocked = errors.New("readiness enrollment blocked")

// TargetEnrollmentEntry is one target to enroll in the readiness transaction.
type TargetEnrollmentEntry struct {
	EnrollmentID     string // new UUID for the target_enrollments row
	TargetID         string // UUID of the target to enroll
	CampaignTargetID string // UUID of the campaign_target row
}

// ReadinessEnrollmentRequest describes everything needed for the readiness transaction.
type ReadinessEnrollmentRequest struct {
	CampaignID       string
	PolicyVersionID  string
	ExpectedRevision int64
	Targets          []TargetEnrollmentEntry // must not be empty
	// Frozen snapshot hashes are written only by the VALIDATING -> READY
	// transition, together with the enrollment claims they authorize.
	FrozenPolicyHash         string
	FrozenTargetSnapshotHash string

	// Audit event fields.
	ActorID       string
	CorrelationID string
	CausationID   string
	Action        string
	ReasonCode    string
	EventSequence int64
	AfterHash     *string

	// Outbox event fields.
	OutboxEventID       string
	OutboxEventType     string
	OutboxSchemaVersion int
	OutboxPayload       json.RawMessage
}

// AcquireReadinessEnrollment executes the readiness enrollment transaction in
// one serializable transaction. It returns ErrEnrollmentBlocked when any target
// is already owned or quarantined.
func AcquireReadinessEnrollment(ctx context.Context, pool *pgxpool.Pool, req ReadinessEnrollmentRequest) error {
	if len(req.Targets) == 0 {
		return fmt.Errorf("readiness enrollment requires at least one target")
	}
	// Sort targets by ID to produce a deterministic lock order (ADR-021).
	targets := make([]TargetEnrollmentEntry, len(req.Targets))
	copy(targets, req.Targets)
	sort.Slice(targets, func(i, j int) bool { return targets[i].TargetID < targets[j].TargetID })

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin readiness enrollment transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // intentional best-effort rollback

	// Step 1: Lock the campaign row to anchor the transaction order.
	var campaignState string
	if err := tx.QueryRow(ctx,
		"SELECT state FROM campaigns WHERE id=$1::uuid FOR UPDATE",
		req.CampaignID,
	).Scan(&campaignState); err != nil {
		return fmt.Errorf("lock campaign row for readiness: %w", err)
	}
	if campaignState != "VALIDATING" {
		return fmt.Errorf("%w: campaign %s is %s, want VALIDATING", ErrRevisionConflict, req.CampaignID, campaignState)
	}
	if req.FrozenPolicyHash == "" || req.FrozenTargetSnapshotHash == "" {
		return fmt.Errorf("readiness enrollment requires frozen policy and target snapshot hashes")
	}

	// Step 2: For each target in sorted ID order, check for blocking
	// conditions then insert enrollment.
	for _, entry := range targets {
		if err := checkAndEnroll(ctx, tx, req.CampaignID, entry); err != nil {
			return err
		}
	}

	// Step 3: Transition campaign to READY with expected-revision guard.
	tag, err := tx.Exec(ctx,
		"UPDATE campaigns SET state='READY', revision=revision+1, "+
			"policy_version_id=$2::uuid, frozen_policy_hash=$3, frozen_target_snapshot_hash=$4, updated_at=now() "+
			"WHERE id=$1::uuid AND revision=$5 AND state='VALIDATING'",
		req.CampaignID, req.PolicyVersionID, req.FrozenPolicyHash, req.FrozenTargetSnapshotHash, req.ExpectedRevision,
	)
	if err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: serialization during readiness transition", ErrRevisionConflict)
		}
		return fmt.Errorf("transition campaign to READY: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: campaign %s at revision %d", ErrRevisionConflict, req.CampaignID, req.ExpectedRevision)
	}

	// Step 4: Append audit event.
	if _, err := tx.Exec(ctx,
		"INSERT INTO audit_events (aggregate_type, aggregate_id, aggregate_revision, event_sequence, "+
			"actor_id, correlation_id, causation_id, action, reason_code, after_hash) "+
			"VALUES ('CAMPAIGN', $1::uuid, $2, $3, $4, $5::uuid, $6::uuid, $7, $8, $9)",
		req.CampaignID, req.ExpectedRevision+1, req.EventSequence,
		req.ActorID, req.CorrelationID, req.CausationID, req.Action, req.ReasonCode, req.AfterHash,
	); err != nil {
		return fmt.Errorf("append readiness audit event: %w", err)
	}

	// Step 5: Insert outbox event.
	if _, err := tx.Exec(ctx,
		"INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, aggregate_revision, "+
			"event_type, schema_version, payload) "+
			"VALUES ($1::uuid, 'CAMPAIGN', $2::uuid, $3, $4, $5, $6)",
		req.OutboxEventID, req.CampaignID, req.ExpectedRevision+1,
		req.OutboxEventType, req.OutboxSchemaVersion, req.OutboxPayload,
	); err != nil {
		return fmt.Errorf("insert readiness outbox event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: serialization during readiness commit", ErrRevisionConflict)
		}
		return fmt.Errorf("commit readiness enrollment: %w", err)
	}
	return nil
}

// checkAndEnroll validates that the target has no blocking active enrollment or
// quarantine, then inserts the enrollment row. It runs within the caller's
// transaction; the sort order guarantees per-target lock order is consistent.
func checkAndEnroll(ctx context.Context, tx pgx.Tx, campaignID string, entry TargetEnrollmentEntry) error {
	// Confirm the caller is claiming the campaign's frozen membership. This also
	// locks the membership row before its enrollment can be created.
	var membershipID string
	if err := tx.QueryRow(ctx,
		"SELECT id FROM campaign_targets WHERE id=$1::uuid AND campaign_id=$2::uuid AND target_id=$3::uuid FOR UPDATE",
		entry.CampaignTargetID, campaignID, entry.TargetID,
	).Scan(&membershipID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: target %s is not a frozen member of campaign %s", ErrEnrollmentBlocked, entry.TargetID, campaignID)
		}
		return fmt.Errorf("lock campaign target membership for %s: %w", entry.TargetID, err)
	}

	// Check for an existing active enrollment (partial index prevents duplicates,
	// but we want an early-exit with a meaningful error rather than a constraint
	// violation to surface the right error type).
	var existingCampaignID string
	err := tx.QueryRow(ctx,
		"SELECT campaign_id FROM target_enrollments WHERE target_id=$1::uuid AND state='ACTIVE' FOR UPDATE",
		entry.TargetID,
	).Scan(&existingCampaignID)
	if err == nil {
		// Another active enrollment exists.
		return fmt.Errorf("%w: target %s already enrolled in campaign %s",
			ErrEnrollmentBlocked, entry.TargetID, existingCampaignID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check active enrollment for target %s: %w", entry.TargetID, err)
	}

	// Check for QUARANTINED operation reservations that block readiness.
	var quarantineID string
	qErr := tx.QueryRow(ctx,
		"SELECT id FROM operation_reservations WHERE target_id=$1::uuid AND state='QUARANTINED' LIMIT 1 FOR UPDATE",
		entry.TargetID,
	).Scan(&quarantineID)
	if qErr == nil {
		return fmt.Errorf("%w: target %s has quarantined operation reservation %s",
			ErrEnrollmentBlocked, entry.TargetID, quarantineID)
	}
	if !errors.Is(qErr, pgx.ErrNoRows) {
		return fmt.Errorf("check quarantine for target %s: %w", entry.TargetID, qErr)
	}

	// Insert the enrollment.
	if _, err := tx.Exec(ctx,
		"INSERT INTO target_enrollments (id, target_id, campaign_id, campaign_target_id) "+
			"VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid)",
		entry.EnrollmentID, entry.TargetID, campaignID, entry.CampaignTargetID,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("%w: target %s enrollment raced another campaign", ErrEnrollmentBlocked, entry.TargetID)
		}
		if isSerialization(err) {
			return fmt.Errorf("%w: target %s enrollment raced another campaign", ErrRevisionConflict, entry.TargetID)
		}
		return fmt.Errorf("insert enrollment for target %s: %w", entry.TargetID, err)
	}
	return nil
}
