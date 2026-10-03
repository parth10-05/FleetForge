//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestDiscoverySuccessAppendsEvidenceAndAdvancesLastGoodHead(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	const targetID = "00000000-0000-0000-2301-000000000001"
	if _, err := pool.Exec(ctx, "INSERT INTO targets (id,external_identity,endpoint_reference,site_key,rack_key,administrative_state) VALUES ($1,'observed-target','endpoint','site','rack','ACTIVE')", targetID); err != nil {
		t.Fatal(err)
	}
	req := persistence.DiscoveryObservationRequest{ObservationID: "00000000-0000-0000-2301-000000000002", TargetID: targetID, ExpectedTargetRevision: 0, ObservedAt: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC), NormalizedData: []byte(`{"redfish_version":"1.16.0","reachable":true}`)}
	if err := persistence.RecordDiscoverySuccess(ctx, pool, req); err != nil {
		t.Fatal(err)
	}
	var latest, good string
	if err := pool.QueryRow(ctx, "SELECT latest_attempt_id,last_good_id FROM target_discovery_heads WHERE target_id=$1", targetID).Scan(&latest, &good); err != nil || latest != req.ObservationID || good != req.ObservationID {
		t.Fatalf("heads=%q,%q err=%v", latest, good, err)
	}
	failureID := "00000000-0000-0000-2301-000000000004"
	if err := persistence.RecordDiscoveryFailure(ctx, pool, persistence.DiscoveryObservationRequest{ObservationID: failureID, TargetID: targetID, ExpectedTargetRevision: 0, ObservedAt: req.ObservedAt.Add(time.Minute), NormalizedData: []byte(`{"error_kind":"timeout"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT latest_attempt_id,last_good_id FROM target_discovery_heads WHERE target_id=$1", targetID).Scan(&latest, &good); err != nil || latest != failureID || good != req.ObservationID {
		t.Fatalf("failure heads=%q,%q err=%v", latest, good, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE targets SET revision=1 WHERE id=$1", targetID); err != nil {
		t.Fatal(err)
	}
	req.ObservationID = "00000000-0000-0000-2301-000000000003"
	if err := persistence.RecordDiscoverySuccess(context.Background(), pool, req); !errors.Is(err, persistence.ErrRevisionConflict) {
		t.Fatalf("stale revision error=%v", err)
	}
}
