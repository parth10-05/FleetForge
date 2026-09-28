package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres/sqlcgen"
)

var ErrPreflightCandidateNotFound = errors.New("preflight candidate not found")

type CandidateDrift struct {
	TargetID string
	Reasons  []string
}

// ReportPreflightCandidateDrift is diagnostic-only. It compares the frozen
// target evidence with live inventory but never mutates snapshot membership.
func ReportPreflightCandidateDrift(ctx context.Context, pool *pgxpool.Pool, candidateID string) ([]CandidateDrift, error) {
	id, err := uuidParameter(candidateID)
	if err != nil {
		return nil, fmt.Errorf("preflight candidate ID: %w", err)
	}
	rows, err := sqlcgen.New(pool).PreflightCandidateDrift(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load preflight candidate drift: %w", err)
	}
	if len(rows) == 0 {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM preflight_candidates WHERE id=$1)`, id).Scan(&exists); err != nil {
			return nil, fmt.Errorf("check preflight candidate: %w", err)
		}
		if !exists {
			return nil, ErrPreflightCandidateNotFound
		}
	}
	result := make([]CandidateDrift, 0)
	for _, row := range rows {
		reasons := make([]string, 0, 5)
		if !row.CurrentRevision.Valid {
			reasons = append(reasons, "target_missing")
		} else {
			if row.CurrentRevision.Int64 != row.FrozenTargetRevision {
				reasons = append(reasons, "revision_changed")
			}
			if !row.CurrentAdministrativeState.Valid || row.CurrentAdministrativeState.String != "ACTIVE" {
				reasons = append(reasons, "administrative_state_changed")
			}
			if !row.CurrentSiteKey.Valid || row.CurrentSiteKey.String != row.FrozenSiteKey {
				reasons = append(reasons, "site_changed")
			}
			if !row.CurrentRackKey.Valid || row.CurrentRackKey.String != row.FrozenRackKey {
				reasons = append(reasons, "rack_changed")
			}
			if !row.CurrentFamily.Valid || row.CurrentFamily.String != row.FrozenFamily {
				reasons = append(reasons, "family_changed")
			}
		}
		if len(reasons) > 0 {
			result = append(result, CandidateDrift{TargetID: row.TargetID.String(), Reasons: reasons})
		}
	}
	return result, nil
}
