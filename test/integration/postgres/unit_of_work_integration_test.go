//go:build postgresintegration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

// ─── P03-07: atomic unit-of-work integration proofs ─────────────────────────

// TestUnitOfWorkAppliesAtomicCampaignMutation proves that a valid
// AggregateMutation commits campaign state, audit event, and outbox event
// atomically in a single short transaction.
func TestUnitOfWorkAppliesAtomicCampaignMutation(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)

	m := campaignMutation(fixture, 0, "VALIDATING", "00000000-0000-0000-7001-000000000001", "00000000-0000-0000-7001-000000000002")
	if err := persistence.ApplyAggregateMutation(ctx, pool, m); err != nil {
		t.Fatalf("ApplyAggregateMutation: %v", err)
	}

	var state string
	var revision int64
	if err := pool.QueryRow(ctx, "SELECT state, revision FROM campaigns WHERE id=$1", fixture.campaignID).Scan(&state, &revision); err != nil {
		t.Fatalf("read campaign after mutation: %v", err)
	}
	if state != "VALIDATING" || revision != 1 {
		t.Errorf("campaign after mutation: state=%q revision=%d, want VALIDATING/1", state, revision)
	}

	var auditCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_events WHERE aggregate_type='CAMPAIGN' AND aggregate_id=$1::uuid", fixture.campaignID).Scan(&auditCount); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("audit events after mutation = %d, want 1", auditCount)
	}

	var outboxCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox_events WHERE aggregate_type='CAMPAIGN' AND aggregate_id=$1::uuid", fixture.campaignID).Scan(&outboxCount); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if outboxCount != 1 {
		t.Errorf("outbox events after mutation = %d, want 1", outboxCount)
	}
}

// TestUnitOfWorkRevisionConflictAllowsOnlyOneWinner proves that two concurrent
// transactions with the same expected revision produce exactly one winner and
// one ErrRevisionConflict.
func TestUnitOfWorkRevisionConflictAllowsOnlyOneWinner(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)

	type result struct{ err error }
	results := make(chan result, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	for i := 0; i < 2; i++ {
		eventID := fmt.Sprintf("00000000-0000-0000-7002-00000000000%d", i+1)
		cmdID := fmt.Sprintf("00000000-0000-0000-8002-00000000000%d", i+1)
		go func(evID, cID string) {
			defer wg.Done()
			m := campaignMutation(fixture, 0, "VALIDATING", evID, cID)
			results <- result{err: persistence.ApplyAggregateMutation(context.Background(), pool, m)}
		}(eventID, cmdID)
	}
	wg.Wait()
	close(results)

	var successes, conflicts int
	for r := range results {
		if r.err == nil {
			successes++
		} else if errors.Is(r.err, persistence.ErrRevisionConflict) {
			conflicts++
		} else {
			t.Errorf("unexpected error: %v", r.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Errorf("concurrent revision outcome: successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}
}

// TestUnitOfWorkRollsBackOnDownstreamWriteFailure proves that when the audit
// event insert fails (simulated by a bad aggregate_type string that violates
// the check constraint), the entire transaction is rolled back and the
// campaign row is unchanged.
func TestUnitOfWorkRollsBackOnDownstreamWriteFailure(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)

	// Craft a mutation with an invalid audit AggregateType to force audit INSERT failure.
	m := campaignMutation(fixture, 0, "VALIDATING", "00000000-0000-0000-7003-000000000001", "00000000-0000-0000-7003-000000000002")
	m.AggregateType = "CORRUPT_TYPE" // violates both the UPDATE switch and constraint

	err := persistence.ApplyAggregateMutation(ctx, pool, m)
	if err == nil {
		t.Fatal("ApplyAggregateMutation with corrupt type succeeded, want error")
	}

	// Campaign row must be unchanged (state=DRAFT, revision=0).
	var state string
	var revision int64
	if dbErr := pool.QueryRow(ctx, "SELECT state, revision FROM campaigns WHERE id=$1", fixture.campaignID).Scan(&state, &revision); dbErr != nil {
		t.Fatalf("read campaign after rollback: %v", dbErr)
	}
	if state != "DRAFT" || revision != 0 {
		t.Errorf("campaign after failed mutation: state=%q revision=%d, want DRAFT/0", state, revision)
	}

	// Audit and outbox must be empty.
	var auditCount, outboxCount int
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_events WHERE aggregate_type='CAMPAIGN' AND aggregate_id=$1::uuid", fixture.campaignID).Scan(&auditCount)
	_ = pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox_events WHERE aggregate_type='CAMPAIGN' AND aggregate_id=$1::uuid", fixture.campaignID).Scan(&outboxCount)
	if auditCount != 0 || outboxCount != 0 {
		t.Errorf("rows after rollback: audit=%d outbox=%d, want 0/0", auditCount, outboxCount)
	}
}

// TestUnitOfWorkSchedulesDueWork proves that an optional DueWorkID causes a
// due_work row to be inserted atomically with the aggregate mutation.
func TestUnitOfWorkSchedulesDueWork(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)

	m := campaignMutation(fixture, 0, "VALIDATING", "00000000-0000-0000-7004-000000000001", "00000000-0000-0000-7004-000000000002")
	m.DueWorkID = "00000000-0000-0000-7004-000000000010"
	m.DueWorkCommandID = "00000000-0000-0000-7004-000000000011"
	m.DueWorkSubjectType = "CAMPAIGN"
	m.DueWorkSubjectID = fixture.campaignID
	m.DueWorkWorkType = "VALIDATE"
	m.DueWorkSchemaVersion = 1
	m.DueWorkNotBefore = time.Now()
	m.DueWorkExpRevision = 1

	if err := persistence.ApplyAggregateMutation(ctx, pool, m); err != nil {
		t.Fatalf("ApplyAggregateMutation with due work: %v", err)
	}

	var workType string
	if err := pool.QueryRow(ctx, "SELECT work_type FROM due_work WHERE id=$1::uuid", m.DueWorkID).Scan(&workType); err != nil {
		t.Fatalf("read due_work row: %v", err)
	}
	if workType != "VALIDATE" {
		t.Errorf("due_work work_type = %q, want VALIDATE", workType)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func campaignMutation(fixture ownershipFixture, revision int64, newState, eventID, commandID string) persistence.AggregateMutation {
	cmdID := commandID
	return persistence.AggregateMutation{
		AggregateType:       "CAMPAIGN",
		AggregateID:         fixture.campaignID,
		ExpectedRevision:    revision,
		NewState:            newState,
		CommandEpoch:        0,
		PolicyVersionID:     "00000000-0000-0000-0000-000000000207",
		ActorID:             "system",
		CommandID:           &cmdID,
		Action:              "TRANSITION",
		ReasonCode:          "campaign.validation.accepted",
		EventSequence:       0,
		OutboxEventID:       eventID,
		OutboxEventType:     "campaign.validation_started",
		OutboxSchemaVersion: 1,
		OutboxPayload:       json.RawMessage(`{"version":"v1"}`),
	}
}
