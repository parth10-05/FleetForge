//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/parth10-05/fleetforge/internal/application/artifacts"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestArtifactVerificationWorkClaimAndLeaseFence(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	const artifactID = "00000000-0000-0000-2611-000000000001"
	const workID = "00000000-0000-0000-2611-000000000002"
	const commandID = "00000000-0000-0000-2611-000000000003"
	const leaseToken = "00000000-0000-0000-2611-000000000004"
	if _, err := pool.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state,revision) VALUES ($1,'v1','sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','objects/a.bin','VERIFYING',3)", artifactID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision) VALUES ($1,$2,'ARTIFACT',$3,'VERIFY_ARTIFACT',1,$4,3)", workID, commandID, artifactID, now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	adapter := persistence.ArtifactVerificationWorkAdapter{Pool: pool}
	claimed, err := adapter.ClaimVerificationWork(ctx, app.VerificationWorkClaimRequest{Owner: "verify-worker", LeaseToken: leaseToken, Now: now, LeaseExpiresAt: now.Add(time.Minute)})
	if err != nil || len(claimed) != 1 || claimed[0].ID != workID || claimed[0].ArtifactID != artifactID {
		t.Fatalf("claimed=%+v err=%v", claimed, err)
	}
	if err := adapter.CompleteVerificationWork(ctx, claimed[0], now); err != nil {
		t.Fatal(err)
	}
	if err := adapter.RetryVerificationWork(context.Background(), claimed[0], now, now.Add(time.Second)); !errors.Is(err, persistence.ErrLeaseConflict) {
		t.Fatalf("stale retry=%v", err)
	}
	var state string
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM due_work WHERE id=$1", workID).Scan(&state); err != nil || state != "COMPLETED" {
		t.Fatalf("state=%q err=%v", state, err)
	}
}
