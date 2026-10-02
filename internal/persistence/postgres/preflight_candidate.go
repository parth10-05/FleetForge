package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/preflight"
)

var ErrPreflightCandidateConflict = errors.New("preflight candidate conflicts with existing immutable snapshot")
var ErrPreflightInputStale = errors.New("preflight candidate input is no longer trusted")

// PreflightCandidateRequest holds only already-evaluated, bounded evidence.
// Report must be a JSON object without credentials or raw device documents.
type PreflightCandidateRequest struct {
	CandidateID string
	Snapshot    preflight.Snapshot
	Report      []byte
}

// PersistPreflightCandidate stores a passed candidate and its exact target
// membership atomically. Existing candidate IDs are immutable: a replay is
// stable only when it names the same snapshot hash.
func PersistPreflightCandidate(ctx context.Context, pool *pgxpool.Pool, request PreflightCandidateRequest) error {
	candidateID, err := uuidParameter(request.CandidateID)
	if err != nil {
		return fmt.Errorf("preflight candidate ID: %w", err)
	}
	if request.Snapshot.Hash().IsZero() || len(request.Report) == 0 || len(request.Report) > 32*1024 || !isJSONObject(request.Report) {
		return fmt.Errorf("invalid preflight candidate request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin preflight candidate: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := validateCandidateInputs(ctx, tx, request.Snapshot); err != nil {
		return err
	}
	command := `INSERT INTO preflight_candidates (id, artifact_id, artifact_version_id, selector_version, selector_json, snapshot_hash, state, report) VALUES ($1, $2::uuid, $3, $4, $5::jsonb, $6, 'PASSED', $7::jsonb) ON CONFLICT (id) DO NOTHING RETURNING id`
	var inserted string
	err = tx.QueryRow(ctx, command, candidateID, request.Snapshot.ArtifactID().String(), request.Snapshot.ObjectIdentity().VersionID(), request.Snapshot.SelectorVersion(), string(request.Snapshot.Selector()), request.Snapshot.Hash().String(), string(request.Report)).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingHash string
		if err := tx.QueryRow(ctx, `SELECT snapshot_hash FROM preflight_candidates WHERE id=$1 FOR KEY SHARE`, candidateID).Scan(&existingHash); err != nil {
			return fmt.Errorf("read preflight candidate replay: %w", err)
		}
		if existingHash != request.Snapshot.Hash().String() {
			return ErrPreflightCandidateConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit preflight candidate replay: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("insert preflight candidate: %w", err)
	}
	for _, target := range request.Snapshot.Targets() {
		if _, err := tx.Exec(ctx, `INSERT INTO preflight_candidate_targets (candidate_id, target_id, frozen_target_revision, site_key, rack_key, target_family, compatibility_policy_version, compatibility_reason) VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8)`, candidateID, target.ID.String(), int64(target.Revision.Uint64()), target.Labels.Site().String(), target.Labels.Rack().String(), target.Family.String(), target.Compatibility.PolicyVersion(), target.Compatibility.Reason()); err != nil {
			return fmt.Errorf("insert preflight candidate target %s: %w", target.ID.String(), err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		if isSerialization(err) {
			return fmt.Errorf("%w: candidate commit", ErrPreflightCandidateConflict)
		}
		return fmt.Errorf("commit preflight candidate: %w", err)
	}
	return nil
}

func validateCandidateInputs(ctx context.Context, tx pgx.Tx, snapshot preflight.Snapshot) error {
	object := snapshot.ObjectIdentity()
	var state, provider, container, key, versionID, digest string
	var size int64
	if err := tx.QueryRow(ctx, `SELECT a.state, o.provider, o.container, o.object_key, o.version_id, o.object_size, o.sha256_digest FROM firmware_artifacts a JOIN artifact_object_versions o ON o.artifact_id = a.id WHERE a.id=$1::uuid FOR SHARE OF a, o`, snapshot.ArtifactID().String()).Scan(&state, &provider, &container, &key, &versionID, &size, &digest); err != nil {
		return fmt.Errorf("validate preflight artifact: %w", err)
	}
	if state != "VERIFIED" || provider != object.Provider() || container != object.Container() || key != object.Key() || versionID != object.VersionID() || size != object.Size() || digest != object.Digest().String() {
		return ErrPreflightInputStale
	}
	for _, target := range snapshot.Targets() {
		var revision int64
		var administrativeState, site, rack, family string
		var trusted bool
		if err := tx.QueryRow(ctx, `SELECT t.revision, t.administrative_state, t.trust_metadata_complete, t.site_key, t.rack_key, l.value FROM targets t JOIN target_labels l ON l.target_id=t.id AND l.key='family' WHERE t.id=$1::uuid FOR SHARE OF t`, target.ID.String()).Scan(&revision, &administrativeState, &trusted, &site, &rack, &family); err != nil {
			return fmt.Errorf("validate preflight target %s: %w", target.ID.String(), err)
		}
		if revision < 0 || uint64(revision) != target.Revision.Uint64() || administrativeState != "ACTIVE" || !trusted || site != target.Labels.Site().String() || rack != target.Labels.Rack().String() || family != target.Family.String() {
			return fmt.Errorf("%w: target %s", ErrPreflightInputStale, target.ID.String())
		}
		if err := validateCurrentDiscoveryEvidence(ctx, tx, target.ID.String(), revision); err != nil {
			return err
		}
	}
	return nil
}

func validateCurrentDiscoveryEvidence(ctx context.Context, tx pgx.Tx, targetID string, revision int64) error {
	var configurationRevision int64
	var latestAttemptID, lastGoodID string
	var fresh bool
	err := tx.QueryRow(ctx, `SELECT head.configuration_revision, head.latest_attempt_id::text, head.last_good_id::text, good.observed_at >= now() - interval '120 seconds' AND good.observed_at <= now() + interval '1 second' FROM target_discovery_heads head JOIN device_observations good ON good.id=head.last_good_id WHERE head.target_id=$1::uuid FOR SHARE OF head`, targetID).Scan(&configurationRevision, &latestAttemptID, &lastGoodID, &fresh)
	if err != nil {
		return fmt.Errorf("%w: target %s has no successful discovery evidence: %v", ErrPreflightInputStale, targetID, err)
	}
	if configurationRevision != revision || latestAttemptID != lastGoodID {
		return fmt.Errorf("%w: target %s discovery evidence is stale or latest attempt failed", ErrPreflightInputStale, targetID)
	}
	if !fresh {
		return fmt.Errorf("%w: target %s discovery evidence is not fresh", ErrPreflightInputStale, targetID)
	}
	return nil
}

func isJSONObject(raw []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value map[string]json.RawMessage
	if err := decoder.Decode(&value); err != nil || value == nil {
		return false
	}
	return decoder.Decode(&struct{}{}) == io.EOF
}
