// due_work.go implements durable work leases only. A lease is delivery
// ownership, never an operation reservation or a firmware-side-effect grant.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrLeaseConflict = errors.New("due-work lease conflict")

type ClaimDueWorkRequest struct {
	Owner          string
	Now            time.Time
	LeaseExpiresAt time.Time
	Tokens         []string
}

type ClaimedDueWork struct {
	ID, CommandID, SubjectType, SubjectID, WorkType, LeaseToken string
	ExpectedRevision                                            int64
	ExpectedCommandEpoch                                        *int64
	DeliveryGeneration                                          int64
}

type DueWorkResult struct {
	ID, LeaseToken       string
	ExpectedRevision     int64
	ExpectedCommandEpoch *int64
	Now                  time.Time
	RetryAt              time.Time
}

// ClaimDueWork atomically turns oldest due PENDING rows into fenced CLAIMED
// rows. Tokens are supplied by the caller to make tests deterministic and keep
// randomness outside the transaction boundary.
func ClaimDueWork(ctx context.Context, pool *pgxpool.Pool, req ClaimDueWorkRequest) ([]ClaimedDueWork, error) {
	if req.Owner == "" || req.Now.IsZero() || !req.LeaseExpiresAt.After(req.Now) || len(req.Tokens) == 0 || len(req.Tokens) > 100 {
		return nil, fmt.Errorf("invalid due-work claim request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin due-work claim: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT id, command_id, subject_type, subject_id, work_type, expected_revision, expected_command_epoch, delivery_generation
FROM due_work WHERE delivery_state='PENDING' AND not_before <= $1 ORDER BY not_before, created_at, id LIMIT $2 FOR UPDATE SKIP LOCKED`, req.Now, len(req.Tokens))
	if err != nil {
		return nil, classifyDueWorkError(err, "lock due work")
	}
	defer rows.Close()
	claimed := make([]ClaimedDueWork, 0, len(req.Tokens))
	for rows.Next() {
		var item ClaimedDueWork
		if err := rows.Scan(&item.ID, &item.CommandID, &item.SubjectType, &item.SubjectID, &item.WorkType, &item.ExpectedRevision, &item.ExpectedCommandEpoch, &item.DeliveryGeneration); err != nil {
			return nil, fmt.Errorf("scan due work: %w", err)
		}
		claimed = append(claimed, item)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyDueWorkError(err, "read due work")
	}
	for i := range claimed {
		claimed[i].LeaseToken = req.Tokens[i]
		tag, err := tx.Exec(ctx, `UPDATE due_work SET delivery_state='CLAIMED', lease_token=$2::uuid, lease_owner=$3, lease_expires_at=$4, updated_at=$5 WHERE id=$1::uuid AND delivery_state='PENDING'`, claimed[i].ID, claimed[i].LeaseToken, req.Owner, req.LeaseExpiresAt, req.Now)
		if err != nil {
			return nil, classifyDueWorkError(err, "claim due work")
		}
		if tag.RowsAffected() != 1 {
			return nil, fmt.Errorf("%w: lost claimed work row", ErrLeaseConflict)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, classifyDueWorkError(err, "commit due-work claim")
	}
	return claimed, nil
}

func CompleteDueWork(ctx context.Context, pool *pgxpool.Pool, result DueWorkResult) error {
	return finishDueWork(ctx, pool, result, true)
}

func FailDueWork(ctx context.Context, pool *pgxpool.Pool, result DueWorkResult) error {
	if result.RetryAt.IsZero() || result.Now.IsZero() || result.RetryAt.Before(result.Now) {
		return fmt.Errorf("invalid due-work retry time")
	}
	return finishDueWork(ctx, pool, result, false)
}

func finishDueWork(ctx context.Context, pool *pgxpool.Pool, r DueWorkResult, complete bool) error {
	if r.ID == "" || r.LeaseToken == "" || r.ExpectedRevision < 0 || r.Now.IsZero() {
		return fmt.Errorf("invalid due-work result")
	}
	var tag pgconnCommandTag
	var err error
	if complete {
		tag, err = pool.Exec(ctx, `UPDATE due_work SET delivery_state='COMPLETED', lease_token=NULL, lease_owner=NULL, lease_expires_at=NULL, updated_at=$5 WHERE id=$1::uuid AND delivery_state='CLAIMED' AND lease_token=$2::uuid AND expected_revision=$3 AND expected_command_epoch IS NOT DISTINCT FROM $4`, r.ID, r.LeaseToken, r.ExpectedRevision, r.ExpectedCommandEpoch, r.Now)
	} else {
		tag, err = pool.Exec(ctx, `UPDATE due_work SET delivery_state='PENDING', not_before=$5, lease_token=NULL, lease_owner=NULL, lease_expires_at=NULL, updated_at=$6 WHERE id=$1::uuid AND delivery_state='CLAIMED' AND lease_token=$2::uuid AND expected_revision=$3 AND expected_command_epoch IS NOT DISTINCT FROM $4`, r.ID, r.LeaseToken, r.ExpectedRevision, r.ExpectedCommandEpoch, r.RetryAt, r.Now)
	}
	if err != nil {
		return classifyDueWorkError(err, "finish due work")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: stale token, revision, or epoch", ErrLeaseConflict)
	}
	return nil
}

// RepairExpiredDueWork never resubmits. SUBMIT work becomes reconciliation
// required, while non-submit work becomes pending with the same command ID and
// generation. Neither path touches admissions, reservations, or capacity.
func RepairExpiredDueWork(ctx context.Context, pool *pgxpool.Pool, now time.Time, limit int) ([]string, error) {
	if now.IsZero() || limit <= 0 || limit > 100 {
		return nil, fmt.Errorf("invalid due-work repair request")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return nil, fmt.Errorf("begin due-work repair: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT id, work_type FROM due_work WHERE delivery_state='CLAIMED' AND lease_expires_at <= $1 ORDER BY lease_expires_at, created_at, id LIMIT $2 FOR UPDATE SKIP LOCKED`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("lock expired due work: %w", err)
	}
	type expiredDueWork struct {
		id       string
		workType string
	}
	expired := []expiredDueWork{}
	for rows.Next() {
		var id, workType string
		if err := rows.Scan(&id, &workType); err != nil {
			return nil, fmt.Errorf("scan expired due work: %w", err)
		}
		expired = append(expired, expiredDueWork{id: id, workType: workType})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read expired due work: %w", err)
	}
	// pgx permits only one active command per connection. Release the locked
	// result cursor before issuing the state-transition updates below; the
	// transaction keeps its FOR UPDATE locks until commit.
	rows.Close()

	ids := make([]string, 0, len(expired))
	for _, work := range expired {
		state := "PENDING"
		if work.workType == "SUBMIT" {
			state = "RECONCILIATION_REQUIRED"
		}
		if _, err := tx.Exec(ctx, `UPDATE due_work SET delivery_state=$2, lease_token=NULL, lease_owner=NULL, lease_expires_at=NULL, updated_at=$3 WHERE id=$1::uuid AND delivery_state='CLAIMED'`, work.id, state, now); err != nil {
			return nil, classifyDueWorkError(err, "repair expired due work")
		}
		ids = append(ids, work.id)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, classifyDueWorkError(err, "commit due-work repair")
	}
	return ids, nil
}

// pgx exposes a command tag interface; keep it local to avoid leaking pgx
// values in public request/result contracts.
type pgconnCommandTag interface{ RowsAffected() int64 }

func classifyDueWorkError(err error, operation string) error {
	if isSerialization(err) {
		return fmt.Errorf("%w: %s", ErrLeaseConflict, operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}
