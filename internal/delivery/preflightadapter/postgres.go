// Package preflightadapter adapts PostgreSQL facts to the preflight application ports.
package preflightadapter

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	application "github.com/parth10-05/fleetforge/internal/application/preflight"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
	"github.com/parth10-05/fleetforge/internal/domain/preflight"
	"github.com/parth10-05/fleetforge/internal/domain/value"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

type PostgresPorts struct{ Pool *pgxpool.Pool }

func (ports PostgresPorts) Select(ctx context.Context, selector inventory.Selector, maximum int) ([]application.Target, error) {
	selected, err := persistence.SelectTargets(ctx, ports.Pool, selector, maximum)
	if err != nil {
		return nil, err
	}
	result := make([]application.Target, 0, len(selected))
	for _, target := range selected {
		result = append(result, application.Target{ID: target.ID, Revision: target.Revision, Labels: target.Labels, Family: target.Family})
	}
	return result, nil
}

func (ports PostgresPorts) Artifact(ctx context.Context, artifactID value.ArtifactID) (application.Artifact, error) {
	loaded, err := persistence.LoadPreflightArtifact(ctx, ports.Pool, artifactID)
	if err != nil {
		return application.Artifact{}, err
	}
	return application.Artifact{ID: loaded.ID, State: loaded.State, Family: loaded.Family, Object: loaded.Object}, nil
}

func (ports PostgresPorts) Persist(ctx context.Context, candidateID string, snapshot preflight.Snapshot, report []byte) error {
	if err := persistence.PersistPreflightCandidate(ctx, ports.Pool, persistence.PreflightCandidateRequest{CandidateID: candidateID, Snapshot: snapshot, Report: report}); err != nil {
		return fmt.Errorf("persist candidate: %w", err)
	}
	return nil
}

var _ application.TargetSelector = PostgresPorts{}
var _ application.ArtifactReader = PostgresPorts{}
var _ application.CandidateWriter = PostgresPorts{}
