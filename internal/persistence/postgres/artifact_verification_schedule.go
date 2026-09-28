package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres/sqlcgen"
)

// ArtifactVerificationScheduleRequest carries only durable worker intent. A
// verification worker must claim the row before it opens the bound object.
type ArtifactVerificationScheduleRequest struct {
	WorkID, CommandID, ArtifactID string
	ArtifactRevision              int64
	NotBefore                     time.Time
}

// ScheduleArtifactVerification creates one active verification job for an
// artifact revision. scheduled=false is a stable idempotent result.
func ScheduleArtifactVerification(ctx context.Context, pool *pgxpool.Pool, request ArtifactVerificationScheduleRequest) (scheduled bool, err error) {
	if request.ArtifactRevision < 0 || request.NotBefore.IsZero() {
		return false, fmt.Errorf("invalid artifact verification schedule request")
	}
	workID, err := uuidParameter(request.WorkID)
	if err != nil {
		return false, fmt.Errorf("artifact verification work id: %w", err)
	}
	commandID, err := uuidParameter(request.CommandID)
	if err != nil {
		return false, fmt.Errorf("artifact verification command id: %w", err)
	}
	artifactID, err := uuidParameter(request.ArtifactID)
	if err != nil {
		return false, fmt.Errorf("artifact verification artifact id: %w", err)
	}
	row, err := sqlcgen.New(pool).ScheduleArtifactVerification(ctx, sqlcgen.ScheduleArtifactVerificationParams{
		ID:               workID,
		CommandID:        commandID,
		SubjectID:        artifactID,
		NotBefore:        pgtype.Timestamptz{Time: request.NotBefore.UTC(), Valid: true},
		ExpectedRevision: request.ArtifactRevision,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("schedule artifact verification: %w", err)
	}
	if !row.Valid {
		return false, fmt.Errorf("schedule artifact verification: invalid returned id")
	}
	return true, nil
}
