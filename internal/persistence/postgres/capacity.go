package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CapacityAvailable reads the existing durable capacity truth in its accepted
// GLOBAL -> SITE -> RACK lock order. It creates neither an admission nor a
// reservation, and therefore cannot authorize a firmware submission.
func CapacityAvailable(ctx context.Context, pool *pgxpool.Pool, global, site, rack ScopeCapacity) (bool, error) {
	if pool == nil {
		return false, fmt.Errorf("capacity pool is required")
	}
	for _, scope := range []ScopeCapacity{global, site, rack} {
		if scope.Limit <= 0 || scope.Key == "" || (scope.Level != "GLOBAL" && scope.Level != "SITE" && scope.Level != "RACK") || (scope.Level == "GLOBAL" && scope.Key != "GLOBAL") {
			return false, fmt.Errorf("invalid capacity scope")
		}
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return false, fmt.Errorf("begin capacity check: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, scope := range []ScopeCapacity{global, site, rack} {
		var limit, active int64
		err := tx.QueryRow(ctx, `SELECT capacity_limit,active_reservation_count FROM scope_capacities WHERE scope_level=$1 AND scope_key=$2 FOR KEY SHARE`, scope.Level, scope.Key).Scan(&limit, &active)
		if err != nil {
			return false, fmt.Errorf("read capacity %s/%s: %w", scope.Level, scope.Key, err)
		}
		if limit != scope.Limit {
			return false, fmt.Errorf("%w: %s/%s", ErrCapacityConfiguration, scope.Level, scope.Key)
		}
		if active >= limit {
			return false, nil
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit capacity check: %w", err)
	}
	return true, nil
}
