//go:build postgresintegration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

// ─── P03-09: readiness enrollment transaction integration proofs ─────────────

// TestReadinessEnrollmentTransitionsCampaignToReady proves the happy path:
// a single target enrolled successfully transitions the campaign to READY
// and creates both an audit event and an outbox event atomically.
func TestReadinessEnrollmentTransitionsCampaignToReady(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareReadinessCampaign(t, pool, ctx, fixture)

	req := enrollmentRequest(fixture, "00000000-0000-0000-9001-000000000001", "00000000-0000-0000-9001-000000000002")
	if err := persistence.AcquireReadinessEnrollment(ctx, pool, req); err != nil {
		t.Fatalf("AcquireReadinessEnrollment: %v", err)
	}

	var state string
	var revision int64
	if err := pool.QueryRow(ctx, "SELECT state, revision FROM campaigns WHERE id=$1", fixture.campaignID).Scan(&state, &revision); err != nil {
		t.Fatalf("read campaign: %v", err)
	}
	if state != "READY" || revision != 1 {
		t.Errorf("campaign after readiness: state=%q revision=%d, want READY/1", state, revision)
	}

	var enrollmentState string
	if err := pool.QueryRow(ctx, "SELECT state FROM target_enrollments WHERE target_id=$1", fixture.targetID).Scan(&enrollmentState); err != nil {
		t.Fatalf("read enrollment: %v", err)
	}
	if enrollmentState != "ACTIVE" {
		t.Errorf("enrollment state = %q, want ACTIVE", enrollmentState)
	}

	var auditCount, outboxCount int
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_events WHERE aggregate_type='CAMPAIGN' AND aggregate_id=$1::uuid", fixture.campaignID).Scan(&auditCount)
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox_events WHERE aggregate_type='CAMPAIGN' AND aggregate_id=$1::uuid", fixture.campaignID).Scan(&outboxCount)
	if auditCount != 1 || outboxCount != 1 {
		t.Errorf("audit=%d outbox=%d, want 1/1", auditCount, outboxCount)
	}
}

// TestReadinessEnrollmentBlockedByExistingActiveEnrollment proves that when a
// target already has an active enrollment, AcquireReadinessEnrollment returns
// ErrEnrollmentBlocked and the campaign remains unchanged.
func TestReadinessEnrollmentBlockedByExistingActiveEnrollment(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareReadinessCampaign(t, pool, ctx, fixture)
	// Pre-insert an active enrollment for the target under a different campaign context.
	insertEnrollment(t, pool, ctx, "00000000-0000-0000-9002-000000000001", fixture)

	req := enrollmentRequest(fixture, "00000000-0000-0000-9002-000000000010", "00000000-0000-0000-9002-000000000011")
	err := persistence.AcquireReadinessEnrollment(ctx, pool, req)
	if !errors.Is(err, persistence.ErrEnrollmentBlocked) {
		t.Errorf("already-enrolled target error = %v, want ErrEnrollmentBlocked", err)
	}

	// Campaign must remain unchanged (VALIDATING, revision=0).
	var state string
	var revision int64
	_ = pool.QueryRow(ctx, "SELECT state, revision FROM campaigns WHERE id=$1", fixture.campaignID).Scan(&state, &revision)
	if state != "VALIDATING" || revision != 0 {
		t.Errorf("campaign after blocked enrollment: state=%q revision=%d, want VALIDATING/0", state, revision)
	}
}

// TestReadinessEnrollmentBlockedByUnresolvedReservation proves that neither a
// live nor quarantined physical-operation reservation can be bypassed by a
// later readiness transaction.
func TestReadinessEnrollmentBlockedByUnresolvedReservation(t *testing.T) {
	for _, reservationState := range []string{"ACTIVE", "QUARANTINED"} {
		t.Run(reservationState, func(t *testing.T) {
			_, pool, ctx := migratedHarness(t)
			fixture := seedOwnershipFixture(t, pool, ctx)
			prepareReadinessCampaign(t, pool, ctx, fixture)
			admissionID := "00000000-0000-0000-9003-000000000001"
			insertAdmission(t, pool, ctx, admissionID, "00000000-0000-0000-9003-000000000002", fixture)
			if _, err := pool.Exec(ctx,
				"INSERT INTO operation_reservations (id, admission_id, target_id, campaign_id, scope_level, scope_key, state) "+
					"VALUES ('00000000-0000-0000-9003-000000000010', $1, $2, $3, 'GLOBAL', 'GLOBAL', $4)",
				admissionID, fixture.targetID, fixture.campaignID, reservationState,
			); err != nil {
				t.Fatalf("insert %s reservation: %v", reservationState, err)
			}
			req := enrollmentRequest(fixture, "00000000-0000-0000-9003-000000000020", "00000000-0000-0000-9003-000000000021")
			if err := persistence.AcquireReadinessEnrollment(ctx, pool, req); !errors.Is(err, persistence.ErrEnrollmentBlocked) {
				t.Errorf("%s target error = %v, want ErrEnrollmentBlocked", reservationState, err)
			}
		})
	}
}

// TestReadinessEnrollmentCompetingCampaignsOnlyOneWins proves that when two
// campaigns compete to enroll the same target, exactly one succeeds and the
// other receives ErrEnrollmentBlocked or ErrRevisionConflict.
func TestReadinessEnrollmentCompetingCampaignsOnlyOneWins(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	second := seedSecondCampaignTarget(t, pool, ctx, fixture)
	prepareReadinessCampaign(t, pool, ctx, fixture)
	prepareReadinessCampaign(t, pool, ctx, second)

	type result struct{ err error }
	results := make(chan result, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		req := enrollmentRequest(fixture, "00000000-0000-0000-9004-000000000001", "00000000-0000-0000-9004-000000000002")
		results <- result{err: persistence.AcquireReadinessEnrollment(context.Background(), pool, req)}
	}()
	go func() {
		defer wg.Done()
		secondFixture := ownershipFixture{
			targetID: fixture.targetID, campaignID: second.campaignID,
			campaignTargetID: second.campaignTargetID, targetUpdateID: fixture.targetUpdateID,
			artifactID: fixture.artifactID, admissionID: fixture.admissionID,
		}
		req := enrollmentRequest(secondFixture, "00000000-0000-0000-9004-000000000010", "00000000-0000-0000-9004-000000000011")
		results <- result{err: persistence.AcquireReadinessEnrollment(context.Background(), pool, req)}
	}()
	wg.Wait()
	close(results)

	var successes, blocked int
	for r := range results {
		if r.err == nil {
			successes++
		} else if errors.Is(r.err, persistence.ErrEnrollmentBlocked) || errors.Is(r.err, persistence.ErrRevisionConflict) {
			blocked++
		} else {
			t.Errorf("unexpected error: %v", r.err)
		}
	}
	if successes != 1 || blocked != 1 {
		t.Errorf("competing campaigns: successes=%d blocked=%d, want 1/1", successes, blocked)
	}
}

// TestReadinessEnrollmentTerminalTargetUpdateDoesNotReleaseEnrollment proves
// that marking a target_update as SUCCEEDED does not remove the active
// enrollment (TTL or terminality cannot release a safety claim).
func TestReadinessEnrollmentTerminalTargetUpdateDoesNotReleaseEnrollment(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	prepareReadinessCampaign(t, pool, ctx, fixture)

	req := enrollmentRequest(fixture, "00000000-0000-0000-9005-000000000001", "00000000-0000-0000-9005-000000000002")
	if err := persistence.AcquireReadinessEnrollment(ctx, pool, req); err != nil {
		t.Fatalf("AcquireReadinessEnrollment: %v", err)
	}

	// Mark target_update SUCCEEDED (terminal).
	if _, err := pool.Exec(ctx, "UPDATE target_updates SET state='SUCCEEDED' WHERE id=$1", fixture.targetUpdateID); err != nil {
		t.Fatalf("mark terminal: %v", err)
	}

	var enrollmentState string
	if err := pool.QueryRow(ctx, "SELECT state FROM target_enrollments WHERE target_id=$1", fixture.targetID).Scan(&enrollmentState); err != nil {
		t.Fatalf("read enrollment: %v", err)
	}
	if enrollmentState != "ACTIVE" {
		t.Errorf("enrollment after terminal target update = %q, want ACTIVE", enrollmentState)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func enrollmentRequest(fixture ownershipFixture, enrollmentID, outboxEventID string) persistence.ReadinessEnrollmentRequest {
	return persistence.ReadinessEnrollmentRequest{
		CampaignID:               fixture.campaignID,
		PolicyVersionID:          "00000000-0000-0000-0000-000000000207",
		ExpectedRevision:         0,
		FrozenPolicyHash:         "policy-hash",
		FrozenTargetSnapshotHash: "target-snapshot-hash",
		Targets: []persistence.TargetEnrollmentEntry{{
			EnrollmentID:     enrollmentID,
			TargetID:         fixture.targetID,
			CampaignTargetID: fixture.campaignTargetID,
		}},
		ActorID:             "system",
		CorrelationID:       "00000000-0000-0000-9000-000000000001",
		CausationID:         "00000000-0000-0000-9000-000000000001",
		Action:              "READINESS_ACQUIRED",
		ReasonCode:          "campaign.validation.accepted",
		EventSequence:       0,
		OutboxEventID:       outboxEventID,
		OutboxEventType:     "campaign.validation_succeeded",
		OutboxSchemaVersion: 1,
		OutboxPayload:       json.RawMessage(`{"version":"v1"}`),
	}
}

func prepareReadinessCampaign(t *testing.T, pool *pgxpool.Pool, ctx context.Context, fixture ownershipFixture) {
	t.Helper()
	if _, err := pool.Exec(ctx, "UPDATE campaigns SET state='VALIDATING' WHERE id=$1::uuid", fixture.campaignID); err != nil {
		t.Fatalf("prepare campaign for readiness: %v", err)
	}
}
