package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/parth10-05/fleetforge/internal/application/preflight"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
	"github.com/parth10-05/fleetforge/internal/domain/preflight"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

// PreflightAdapter exposes the existing bounded PostgreSQL primitives through
// the application ports. It owns no eligibility policy of its own.
type PreflightAdapter struct{ Pool *pgxpool.Pool }

func (adapter PreflightAdapter) Select(ctx context.Context, selector inventory.Selector, maximum int) ([]app.Target, error) {
	if adapter.Pool == nil {
		return nil, fmt.Errorf("preflight persistence is unavailable")
	}
	selected, err := SelectTargets(ctx, adapter.Pool, selector, maximum)
	if err != nil {
		return nil, err
	}
	result := make([]app.Target, 0, len(selected))
	for _, target := range selected {
		result = append(result, app.Target{ID: target.ID, Revision: target.Revision, Labels: target.Labels, Family: target.Family})
	}
	return result, nil
}

func (adapter PreflightAdapter) Artifact(ctx context.Context, id value.ArtifactID) (app.Artifact, error) {
	if adapter.Pool == nil {
		return app.Artifact{}, fmt.Errorf("preflight persistence is unavailable")
	}
	record, err := LoadPreflightArtifact(ctx, adapter.Pool, id)
	if err != nil {
		return app.Artifact{}, err
	}
	return app.Artifact{ID: record.ID, State: record.State, Family: record.Family, Object: record.Object}, nil
}

func (adapter PreflightAdapter) Persist(ctx context.Context, candidateID string, snapshot preflight.Snapshot, report []byte) error {
	if adapter.Pool == nil {
		return fmt.Errorf("preflight persistence is unavailable")
	}
	return PersistPreflightCandidate(ctx, adapter.Pool, PreflightCandidateRequest{CandidateID: candidateID, Snapshot: snapshot, Report: report})
}
