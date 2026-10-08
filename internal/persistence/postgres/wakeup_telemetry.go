package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WakeupTelemetrySnapshot is the bounded, identifier-free current diagnostic
// state of the durable wakeup path. It never participates in work authority.
type WakeupTelemetrySnapshot struct {
	ObservedAt                     time.Time
	PendingWakeupCount             int64
	OldestWakeupAgeSeconds         int64
	PendingDueWorkCount            int64
	OldestDueWorkAgeSeconds        int64
	ExpiredDeliveryClaimCount      int64
	ExpiredWorkLeaseCount          int64
	ActiveReservationCount         int64
	RetryExhaustedWakeupCount      int64
	OpenQuarantineCount            int64
	OldestOpenQuarantineAgeSeconds int64
}

// CollectWakeupTelemetry persists the current bounded diagnostic snapshot only
// after callers have completed any repair transaction. It neither claims work
// nor changes a workflow, reservation, or delivery state.
func CollectWakeupTelemetry(ctx context.Context, pool *pgxpool.Pool, now time.Time) (WakeupTelemetrySnapshot, error) {
	if pool == nil || now.IsZero() || now.Location() != time.UTC {
		return WakeupTelemetrySnapshot{}, fmt.Errorf("invalid wakeup telemetry request")
	}
	snapshot := WakeupTelemetrySnapshot{ObservedAt: now}
	err := pool.QueryRow(ctx, `SELECT
    (SELECT COUNT(*) FROM outbox_events WHERE delivery_kind='WORK_WAKEUP' AND delivery_state='PENDING'),
    COALESCE((SELECT GREATEST(0, EXTRACT(EPOCH FROM ($1::timestamptz - MIN(created_at)))::bigint) FROM outbox_events WHERE delivery_kind='WORK_WAKEUP' AND delivery_state='PENDING'), 0),
    (SELECT COUNT(*) FROM due_work WHERE delivery_state='PENDING'),
    COALESCE((SELECT GREATEST(0, EXTRACT(EPOCH FROM ($1::timestamptz - MIN(created_at)))::bigint) FROM due_work WHERE delivery_state='PENDING'), 0),
    (SELECT COUNT(*) FROM outbox_events WHERE delivery_kind='WORK_WAKEUP' AND delivery_state='CLAIMED' AND claim_expires_at < $1::timestamptz),
    (SELECT COUNT(*) FROM due_work WHERE delivery_state='CLAIMED' AND lease_expires_at < $1::timestamptz),
    (SELECT COUNT(*) FROM operation_reservations WHERE state IN ('ACTIVE','QUARANTINED')),
    (SELECT COUNT(*) FROM outbox_events WHERE delivery_kind='WORK_WAKEUP' AND delivery_state='QUARANTINED' AND last_error_class='PUBLISH_RETRYABLE'),
    (SELECT COUNT(*) FROM wakeup_quarantine WHERE resolution_state='OPEN'),
    COALESCE((SELECT GREATEST(0, EXTRACT(EPOCH FROM ($1::timestamptz - MIN(first_observed_at)))::bigint) FROM wakeup_quarantine WHERE resolution_state='OPEN'), 0)`, now).Scan(
		&snapshot.PendingWakeupCount,
		&snapshot.OldestWakeupAgeSeconds,
		&snapshot.PendingDueWorkCount,
		&snapshot.OldestDueWorkAgeSeconds,
		&snapshot.ExpiredDeliveryClaimCount,
		&snapshot.ExpiredWorkLeaseCount,
		&snapshot.ActiveReservationCount,
		&snapshot.RetryExhaustedWakeupCount,
		&snapshot.OpenQuarantineCount,
		&snapshot.OldestOpenQuarantineAgeSeconds,
	)
	if err != nil {
		return WakeupTelemetrySnapshot{}, fmt.Errorf("read wakeup telemetry: %w", err)
	}
	if !validWakeupTelemetry(snapshot) {
		return WakeupTelemetrySnapshot{}, fmt.Errorf("invalid wakeup telemetry snapshot")
	}
	_, err = pool.Exec(ctx, `INSERT INTO wakeup_telemetry_snapshot (singleton,observed_at,pending_wakeup_count,oldest_wakeup_age_seconds,pending_due_work_count,oldest_due_work_age_seconds,expired_delivery_claim_count,expired_work_lease_count,active_reservation_count,retry_exhausted_wakeup_count,open_quarantine_count,oldest_open_quarantine_age_seconds) VALUES (true,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (singleton) DO UPDATE SET observed_at=EXCLUDED.observed_at,pending_wakeup_count=EXCLUDED.pending_wakeup_count,oldest_wakeup_age_seconds=EXCLUDED.oldest_wakeup_age_seconds,pending_due_work_count=EXCLUDED.pending_due_work_count,oldest_due_work_age_seconds=EXCLUDED.oldest_due_work_age_seconds,expired_delivery_claim_count=EXCLUDED.expired_delivery_claim_count,expired_work_lease_count=EXCLUDED.expired_work_lease_count,active_reservation_count=EXCLUDED.active_reservation_count,retry_exhausted_wakeup_count=EXCLUDED.retry_exhausted_wakeup_count,open_quarantine_count=EXCLUDED.open_quarantine_count,oldest_open_quarantine_age_seconds=EXCLUDED.oldest_open_quarantine_age_seconds`, snapshot.ObservedAt, snapshot.PendingWakeupCount, snapshot.OldestWakeupAgeSeconds, snapshot.PendingDueWorkCount, snapshot.OldestDueWorkAgeSeconds, snapshot.ExpiredDeliveryClaimCount, snapshot.ExpiredWorkLeaseCount, snapshot.ActiveReservationCount, snapshot.RetryExhaustedWakeupCount, snapshot.OpenQuarantineCount, snapshot.OldestOpenQuarantineAgeSeconds)
	if err != nil {
		return WakeupTelemetrySnapshot{}, fmt.Errorf("persist wakeup telemetry: %w", err)
	}
	return snapshot, nil
}

func validWakeupTelemetry(snapshot WakeupTelemetrySnapshot) bool {
	if snapshot.ObservedAt.IsZero() || snapshot.ObservedAt.Location() != time.UTC {
		return false
	}
	for _, value := range []int64{snapshot.PendingWakeupCount, snapshot.OldestWakeupAgeSeconds, snapshot.PendingDueWorkCount, snapshot.OldestDueWorkAgeSeconds, snapshot.ExpiredDeliveryClaimCount, snapshot.ExpiredWorkLeaseCount, snapshot.ActiveReservationCount, snapshot.RetryExhaustedWakeupCount, snapshot.OpenQuarantineCount, snapshot.OldestOpenQuarantineAgeSeconds} {
		if value < 0 {
			return false
		}
	}
	return true
}
