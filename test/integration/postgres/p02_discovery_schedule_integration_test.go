//go:build postgresintegration

package postgres

import (
	"context"
	"testing"
	"time"

	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestP02DiscoverySchedulingDeduplicatesActiveTargetRevision(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	const targetID = "00000000-0000-0000-2201-000000000001"
	if _, err := pool.Exec(ctx, "INSERT INTO targets (id,external_identity,endpoint_reference,site_key,rack_key,administrative_state) VALUES ($1,'discovery-target','endpoint','site','rack','ACTIVE')", targetID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	first, err := persistence.ScheduleDiscovery(ctx, pool, persistence.DiscoveryScheduleRequest{WorkID: "00000000-0000-0000-2201-000000000002", CommandID: "00000000-0000-0000-2201-000000000003", TargetID: targetID, TargetRevision: 4, NotBefore: now})
	if err != nil || !first {
		t.Fatalf("first schedule=%v err=%v", first, err)
	}
	second, err := persistence.ScheduleDiscovery(ctx, pool, persistence.DiscoveryScheduleRequest{WorkID: "00000000-0000-0000-2201-000000000004", CommandID: "00000000-0000-0000-2201-000000000005", TargetID: targetID, TargetRevision: 4, NotBefore: now})
	if err != nil || second {
		t.Fatalf("duplicate schedule=%v err=%v", second, err)
	}
	third, err := persistence.ScheduleDiscovery(context.Background(), pool, persistence.DiscoveryScheduleRequest{WorkID: "00000000-0000-0000-2201-000000000006", CommandID: "00000000-0000-0000-2201-000000000007", TargetID: targetID, TargetRevision: 5, NotBefore: now})
	if err != nil || !third {
		t.Fatalf("new revision schedule=%v err=%v", third, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM due_work WHERE subject_id=$1 AND subject_type='TARGET' AND work_type='DISCOVER'", targetID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("discovery work count=%d err=%v", count, err)
	}
}
