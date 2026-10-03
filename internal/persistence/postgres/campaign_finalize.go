package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/campaign"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

// FinalizeCanaryRequest carries a previously persisted evaluation outcome. The
// caller may request release only for a PASSED evaluation; all other terminal
// outcomes resolve to PAUSED and create no later-wave intent.
type FinalizeCanaryRequest struct {
	CampaignID, TerminalCommandID, ResolutionCommandID string
	ExpectedRevision, ExpectedEpoch                    int64
	Release                                            bool
	Now                                                time.Time
}

// FinalizeCanary records terminal/PASSED state for the canary and advances the
// campaign through the reducer. It is intentionally narrow: P04 owns creation
// and execution of later target-update work.
func FinalizeCanary(ctx context.Context, pool *pgxpool.Pool, r FinalizeCanaryRequest) error {
	if r.ExpectedRevision < 0 || r.ExpectedEpoch < 0 || r.Now.IsZero() {
		return fmt.Errorf("invalid canary finalization")
	}
	for _, raw := range []string{r.CampaignID, r.TerminalCommandID, r.ResolutionCommandID} {
		if _, err := uuidParameter(raw); err != nil {
			return fmt.Errorf("invalid canary finalization identity: %w", err)
		}
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin canary finalization: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var state string
	var revision, epoch int64
	if err := tx.QueryRow(ctx, "SELECT state,revision,command_epoch FROM campaigns WHERE id=$1::uuid FOR UPDATE", r.CampaignID).Scan(&state, &revision, &epoch); err != nil {
		return fmt.Errorf("lock campaign finalization: %w", err)
	}
	if state != "RUNNING_CANARY" || revision != r.ExpectedRevision || epoch != r.ExpectedEpoch {
		return ErrCampaignStartConflict
	}
	first, err := value.ParseCommandID(r.TerminalCommandID)
	if err != nil {
		return err
	}
	second, err := value.ParseCommandID(r.ResolutionCommandID)
	if err != nil {
		return err
	}
	rev, _ := value.NewRevision(revision)
	ep, _ := value.NewCommandEpoch(epoch)
	transitions, ok := campaign.ReduceTerminalCanary(campaign.Aggregate{State: campaign.State(state), Revision: rev, CommandEpoch: ep}, first, second, r.Release)
	if !ok {
		return ErrCampaignStartConflict
	}
	var canaryID, laterID string
	var laterReleased bool
	if err := tx.QueryRow(ctx, "SELECT id::text FROM campaign_waves WHERE campaign_id=$1::uuid AND cohort='CANARY' FOR UPDATE", r.CampaignID).Scan(&canaryID); err != nil {
		return fmt.Errorf("lock canary wave: %w", err)
	}
	if err := tx.QueryRow(ctx, "SELECT id::text,released FROM campaign_waves WHERE campaign_id=$1::uuid AND cohort='LATER_WAVE' FOR UPDATE", r.CampaignID).Scan(&laterID, &laterReleased); err != nil {
		return fmt.Errorf("lock later wave: %w", err)
	}
	if r.Release && laterReleased {
		return ErrCampaignStartConflict
	}
	evaluation := "FAILED"
	if r.Release {
		evaluation = "PASSED"
	}
	if _, err := tx.Exec(ctx, "UPDATE campaign_waves SET terminal=true,evaluation=$2,updated_at=$3 WHERE id=$1::uuid AND terminal=false", canaryID, evaluation, r.Now); err != nil {
		return fmt.Errorf("resolve canary wave: %w", err)
	}
	if r.Release {
		if _, err := tx.Exec(ctx, "UPDATE campaign_waves SET released=true,updated_at=$2 WHERE id=$1::uuid AND released=false", laterID, r.Now); err != nil {
			return fmt.Errorf("release later wave: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, "UPDATE campaigns SET state=$3,revision=$4,command_epoch=$5,suspended_phase=$6,updated_at=$7 WHERE id=$1::uuid AND revision=$2", r.CampaignID, r.ExpectedRevision, transitions.Resolved.Aggregate.State.String(), int64(transitions.Resolved.Aggregate.Revision.Uint64()), int64(transitions.Resolved.Aggregate.CommandEpoch.Uint64()), nullableState(transitions.Resolved.Aggregate.SuspendedPhase), r.Now); err != nil {
		return fmt.Errorf("resolve campaign finalization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit canary finalization: %w", err)
	}
	return nil
}
func nullableState(s campaign.State) any {
	if s == "" {
		return nil
	}
	return s.String()
}
