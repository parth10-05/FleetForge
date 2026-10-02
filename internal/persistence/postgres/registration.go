package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres/sqlcgen"
)

var ErrRegistrationConflict = errors.New("target registration conflicts with existing target")

type RegistrationResult struct {
	TargetID string
	Created  bool
}

// RegisterTarget persists a complete trusted registration. Replaying the same
// semantic request is stable; changing an existing external identity is never
// an implicit update and instead returns ErrRegistrationConflict.
func RegisterTarget(ctx context.Context, pool *pgxpool.Pool, targetID string, registration inventory.Registration) (RegistrationResult, error) {
	id, err := uuidParameter(targetID)
	if err != nil {
		return RegistrationResult{}, fmt.Errorf("target registration id: %w", err)
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return RegistrationResult{}, fmt.Errorf("begin target registration: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	queries := sqlcgen.New(tx)
	labels := registration.Labels()
	row, err := queries.RegisterTrustedTarget(ctx, sqlcgen.RegisterTrustedTargetParams{
		ID: id, ExternalIdentity: registration.ExternalID(), EndpointReference: registration.Address().Origin().String(),
		SiteKey: labels.Site().String(), RackKey: labels.Rack().String(),
		TargetKey:              pgtype.Text{String: registration.Address().TargetKey(), Valid: true},
		CredentialReference:    pgtype.Text{String: registration.CredentialReference(), Valid: true},
		TransportPolicyVersion: pgtype.Text{String: registration.TransportPolicy(), Valid: true},
		RegistrationHash:       pgtype.Text{String: registration.SemanticHash().String(), Valid: true},
	})
	if err == nil {
		resultID, parseErr := sqlcUUIDText(row.ID)
		if parseErr != nil {
			return RegistrationResult{}, parseErr
		}
		for key, label := range labels.AsMap() {
			if _, err := tx.Exec(ctx, `INSERT INTO target_labels (target_id, key, value) VALUES ($1, $2, $3)`, row.ID, key, label); err != nil {
				return RegistrationResult{}, fmt.Errorf("insert target label %q: %w", key, err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return RegistrationResult{}, fmt.Errorf("commit target registration: %w", err)
		}
		return RegistrationResult{TargetID: resultID, Created: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RegistrationResult{}, fmt.Errorf("insert target registration: %w", err)
	}
	existing, err := queries.LockTargetRegistration(ctx, registration.ExternalID())
	if err != nil {
		return RegistrationResult{}, fmt.Errorf("lock existing target registration: %w", err)
	}
	if !existing.TrustMetadataComplete || !existing.RegistrationHash.Valid || existing.RegistrationHash.String != registration.SemanticHash().String() {
		return RegistrationResult{}, ErrRegistrationConflict
	}
	resultID, err := sqlcUUIDText(existing.ID)
	if err != nil {
		return RegistrationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RegistrationResult{}, fmt.Errorf("commit idempotent registration: %w", err)
	}
	return RegistrationResult{TargetID: resultID}, nil
}

func uuidParameter(raw string) (pgtype.UUID, error) {
	var result pgtype.UUID
	if err := result.Scan(raw); err != nil || !result.Valid {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID")
	}
	return result, nil
}
