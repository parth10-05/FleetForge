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

// DiscoveryScheduleRequest carries the durable intent only. The worker does
// all Redfish I/O after a lease has committed; no network action is possible
// from this scheduler.
type DiscoveryScheduleRequest struct {
	WorkID, CommandID, TargetID string
	TargetRevision              int64
	NotBefore                   time.Time
}

// ScheduleDiscovery creates one active discovery job for a target revision.
// Scheduled=false means an equivalent PENDING or CLAIMED job already exists.
func ScheduleDiscovery(ctx context.Context, pool *pgxpool.Pool, request DiscoveryScheduleRequest) (scheduled bool, err error) {
	if request.TargetRevision < 0 || request.NotBefore.IsZero() {
		return false, fmt.Errorf("invalid discovery schedule request")
	}
	workID, err := uuidParameter(request.WorkID)
	if err != nil {
		return false, fmt.Errorf("discovery work id: %w", err)
	}
	commandID, err := uuidParameter(request.CommandID)
	if err != nil {
		return false, fmt.Errorf("discovery command id: %w", err)
	}
	targetID, err := uuidParameter(request.TargetID)
	if err != nil {
		return false, fmt.Errorf("discovery target id: %w", err)
	}
	row, err := sqlcgen.New(pool).ScheduleTargetDiscovery(ctx, sqlcgen.ScheduleTargetDiscoveryParams{
		ID: workID, CommandID: commandID, SubjectID: targetID,
		NotBefore: pgtype.Timestamptz{Time: request.NotBefore.UTC(), Valid: true}, ExpectedRevision: request.TargetRevision,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("schedule target discovery: %w", err)
	}
	if !row.Valid {
		return false, fmt.Errorf("schedule target discovery: invalid returned id")
	}
	return true, nil
}
