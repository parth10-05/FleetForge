package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DiscoveryObservationRequest is already normalized, bounded, and redacted.
// It carries no raw Redfish document, headers, endpoint, or credentials.
type DiscoveryObservationRequest struct {
	ObservationID, TargetID string
	ExpectedTargetRevision  int64
	ObservedAt              time.Time
	NormalizedData          json.RawMessage
	RawEvidenceReference    *string
}

// RecordDiscoverySuccess appends one last-known-good discovery result and
// advances its per-target head only if the target configuration revision is
// still current. The caller must complete network I/O before invoking it.
func RecordDiscoverySuccess(ctx context.Context, pool *pgxpool.Pool, request DiscoveryObservationRequest) error {
	if request.ExpectedTargetRevision < 0 || request.ObservedAt.IsZero() || len(request.NormalizedData) == 0 || len(request.NormalizedData) > 32768 || !json.Valid(request.NormalizedData) {
		return fmt.Errorf("invalid discovery observation request")
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(request.NormalizedData, &object) != nil || object == nil {
		return fmt.Errorf("discovery observation must be a JSON object")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin discovery observation: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var revision int64
	if err := tx.QueryRow(ctx, "SELECT revision FROM targets WHERE id=$1::uuid FOR UPDATE", request.TargetID).Scan(&revision); err != nil {
		return fmt.Errorf("lock discovery target: %w", err)
	}
	if revision != request.ExpectedTargetRevision {
		return fmt.Errorf("%w: discovery target revision changed", ErrRevisionConflict)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO device_observations (id,target_id,kind,observed_at,normalized_data,raw_object_reference) VALUES ($1::uuid,$2::uuid,'INVENTORY',$3,$4,$5)", request.ObservationID, request.TargetID, request.ObservedAt.UTC(), request.NormalizedData, request.RawEvidenceReference); err != nil {
		return fmt.Errorf("insert discovery observation: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO target_discovery_heads (target_id,head_revision,configuration_revision,latest_attempt_id,last_good_id) VALUES ($1::uuid,1,$2,$3::uuid,$3::uuid) ON CONFLICT (target_id) DO UPDATE SET head_revision=target_discovery_heads.head_revision+1,configuration_revision=EXCLUDED.configuration_revision,latest_attempt_id=EXCLUDED.latest_attempt_id,last_good_id=EXCLUDED.last_good_id", request.TargetID, request.ExpectedTargetRevision, request.ObservationID); err != nil {
		return fmt.Errorf("advance discovery head: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: discovery commit", ErrRevisionConflict)
		}
		return fmt.Errorf("commit discovery observation: %w", err)
	}
	return nil
}

// RecordDiscoveryFailure appends normalized failure evidence without erasing
// the target's prior last-known-good observation.
func RecordDiscoveryFailure(ctx context.Context, pool *pgxpool.Pool, request DiscoveryObservationRequest) error {
	if request.ExpectedTargetRevision < 0 || request.ObservedAt.IsZero() || len(request.NormalizedData) == 0 || len(request.NormalizedData) > 32768 || !json.Valid(request.NormalizedData) {
		return fmt.Errorf("invalid discovery observation request")
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(request.NormalizedData, &object) != nil || object == nil {
		return fmt.Errorf("discovery observation must be a JSON object")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin discovery failure: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var revision int64
	if err := tx.QueryRow(ctx, "SELECT revision FROM targets WHERE id=$1::uuid FOR UPDATE", request.TargetID).Scan(&revision); err != nil {
		return fmt.Errorf("lock discovery target: %w", err)
	}
	if revision != request.ExpectedTargetRevision {
		return fmt.Errorf("%w: discovery target revision changed", ErrRevisionConflict)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO device_observations (id,target_id,kind,observed_at,normalized_data,raw_object_reference) VALUES ($1::uuid,$2::uuid,'DISCOVERY_FAILURE',$3,$4,$5)", request.ObservationID, request.TargetID, request.ObservedAt.UTC(), request.NormalizedData, request.RawEvidenceReference); err != nil {
		return fmt.Errorf("insert discovery failure: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO target_discovery_heads (target_id,head_revision,configuration_revision,latest_attempt_id) VALUES ($1::uuid,1,$2,$3::uuid) ON CONFLICT (target_id) DO UPDATE SET head_revision=target_discovery_heads.head_revision+1,configuration_revision=EXCLUDED.configuration_revision,latest_attempt_id=EXCLUDED.latest_attempt_id", request.TargetID, request.ExpectedTargetRevision, request.ObservationID); err != nil {
		return fmt.Errorf("advance discovery failure head: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: discovery failure commit", ErrRevisionConflict)
		}
		return fmt.Errorf("commit discovery failure: %w", err)
	}
	return nil
}
