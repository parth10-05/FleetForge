package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type IntegrityFinding struct{ Kind, Subject string }

// ScanIntegrity is report-only. A finding asks an operator/reconciliation path
// to investigate; it never repairs or changes a durable safety claim.
func ScanIntegrity(ctx context.Context, pool *pgxpool.Pool, now time.Time, limit int) ([]IntegrityFinding, error) {
	if now.IsZero() || limit <= 0 || limit > 1000 {
		return nil, fmt.Errorf("invalid integrity scan request")
	}
	queries := []struct {
		kind, sql string
		args      []any
	}{
		{"COUNTER_MISMATCH", `SELECT c.scope_level||':'||c.scope_key FROM scope_capacities c LEFT JOIN (SELECT scope_level,scope_key,COUNT(*) count FROM operation_reservations WHERE state IN ('ACTIVE','QUARANTINED') GROUP BY scope_level,scope_key) r USING(scope_level,scope_key) WHERE c.active_reservation_count<>COALESCE(r.count,0)`, nil},
		{"ORPHAN_DUE_WORK", `SELECT d.id::text FROM due_work d LEFT JOIN target_updates t ON d.subject_type='TARGET_UPDATE' AND d.subject_id=t.id WHERE d.subject_type='TARGET_UPDATE' AND t.id IS NULL`, nil},
		{"STALE_DUE_LEASE", `SELECT id::text FROM due_work WHERE delivery_state='CLAIMED' AND lease_expires_at <= $1`, []any{now}},
		{"STALE_OUTBOX_CLAIM", `SELECT event_id::text FROM outbox_events WHERE delivery_state='CLAIMED' AND claim_expires_at <= $1`, []any{now}},
		{"AUDIT_SEQUENCE_GAP", `SELECT aggregate_type||':'||aggregate_id::text FROM (SELECT aggregate_type,aggregate_id,event_sequence,LAG(event_sequence) OVER(PARTITION BY aggregate_type,aggregate_id ORDER BY aggregate_revision,event_sequence) previous FROM audit_events) x WHERE previous IS NOT NULL AND event_sequence<>previous+1`, nil},
	}
	findings := []IntegrityFinding{}
	for _, q := range queries {
		rows, err := pool.Query(ctx, q.sql, q.args...)
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", q.kind, err)
		}
		for rows.Next() {
			var subject string
			if err := rows.Scan(&subject); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan %s result: %w", q.kind, err)
			}
			findings = append(findings, IntegrityFinding{Kind: q.kind, Subject: subject})
			if len(findings) >= limit {
				rows.Close()
				return findings, nil
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan %s rows: %w", q.kind, err)
		}
		rows.Close()
	}
	return findings, nil
}
