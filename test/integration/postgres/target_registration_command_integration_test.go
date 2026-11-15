//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestIdempotentTargetRegistrationCommitsItsDurableEnvelope(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	claim := persistence.IdempotencyClaim{
		ID: "00000000-0000-0000-2803-000000000001", ActorID: "user:operator", Route: "/v1/targets", RawKey: "target-registration-key", RequestBody: []byte(`{"target":"rack-a-bmc-01"}`),
		CommandID: "00000000-0000-0000-2803-000000000002", OwnerToken: "00000000-0000-0000-2803-000000000003", TTL: defaultTTL,
	}
	if err := persistence.ClaimIdempotencyKey(ctx, pool, claim); err != nil {
		t.Fatalf("claim idempotency key: %v", err)
	}
	const targetID = "00000000-0000-0000-2803-000000000004"
	reference, err := (persistence.ResponseReference{Operation: "target-register", ResourceID: targetID, Revision: 0, CommandID: claim.CommandID}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	result, err := persistence.ApplyIdempotentTargetRegistration(ctx, pool, persistence.IdempotentTargetRegistration{
		TargetID: targetID, CommandID: claim.CommandID, CorrelationID: "00000000-0000-0000-2803-000000000005", OwnerToken: claim.OwnerToken,
		OutboxEventID: "00000000-0000-0000-2803-000000000006", DecisionInputID: "00000000-0000-0000-2803-000000000007", Registration: mustRegistration(t, "https://simulator.example"),
		Completion: persistence.IdempotencyCompletion{ActorID: claim.ActorID, Route: claim.Route, RawKey: claim.RawKey, ResponseStatus: 201, ResponseReference: reference},
	})
	if err != nil || !result.Created || result.TargetID != targetID {
		t.Fatalf("apply idempotent registration result=%+v err=%v", result, err)
	}
	for _, table := range []string{"audit_events", "decision_inputs", "outbox_events"} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table+" WHERE aggregate_id=$1::uuid", targetID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v, want one durable record", table, count, err)
		}
	}
	status, storedReference, found, err := persistence.GetCompletedIdempotencyRecord(ctx, pool, claim.ActorID, claim.Route, claim.RawKey)
	if err != nil || !found || status != 201 || storedReference != reference {
		t.Fatalf("completed replay status=%d reference=%q found=%v err=%v", status, storedReference, found, err)
	}
	replay, err := persistence.ResolveTargetRegistrationReplay(ctx, pool, storedReference)
	if err != nil || replay.TargetID != targetID || replay.CommandID != claim.CommandID || replay.Revision != 0 || replay.AcceptedAt.IsZero() {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
}

func TestIdempotentTargetRegistrationRejectsExistingIdentityWithAnotherTargetID(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	first := persistence.IdempotencyClaim{ID: "00000000-0000-0000-2804-000000000001", ActorID: "user:operator", Route: "/v1/targets", RawKey: "target-registration-first", RequestBody: []byte(`{"target":"rack-a-bmc-01"}`), CommandID: "00000000-0000-0000-2804-000000000002", OwnerToken: "00000000-0000-0000-2804-000000000003", TTL: defaultTTL}
	if err := persistence.ClaimIdempotencyKey(ctx, pool, first); err != nil {
		t.Fatal(err)
	}
	applyTargetRegistration(t, pool, ctx, first, "00000000-0000-0000-2804-000000000004")
	second := persistence.IdempotencyClaim{ID: "00000000-0000-0000-2804-000000000005", ActorID: "user:operator", Route: "/v1/targets", RawKey: "target-registration-second", RequestBody: []byte(`{"target":"rack-a-bmc-01"}`), CommandID: "00000000-0000-0000-2804-000000000006", OwnerToken: "00000000-0000-0000-2804-000000000007", TTL: defaultTTL}
	if err := persistence.ClaimIdempotencyKey(ctx, pool, second); err != nil {
		t.Fatal(err)
	}
	reference, err := (persistence.ResponseReference{Operation: "target-register", ResourceID: "00000000-0000-0000-2804-000000000008", Revision: 0, CommandID: second.CommandID}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	_, err = persistence.ApplyIdempotentTargetRegistration(ctx, pool, persistence.IdempotentTargetRegistration{TargetID: "00000000-0000-0000-2804-000000000008", CommandID: second.CommandID, CorrelationID: "00000000-0000-0000-2804-000000000009", OwnerToken: second.OwnerToken, OutboxEventID: "00000000-0000-0000-2804-000000000010", DecisionInputID: "00000000-0000-0000-2804-000000000011", Registration: mustRegistration(t, "https://simulator.example"), Completion: persistence.IdempotencyCompletion{ActorID: second.ActorID, Route: second.Route, RawKey: second.RawKey, ResponseStatus: 201, ResponseReference: reference}})
	if !errors.Is(err, persistence.ErrRegistrationConflict) {
		t.Fatalf("registration error=%v, want ErrRegistrationConflict", err)
	}
}

func applyTargetRegistration(t *testing.T, pool *pgxpool.Pool, ctx context.Context, claim persistence.IdempotencyClaim, targetID string) {
	t.Helper()
	reference, err := (persistence.ResponseReference{Operation: "target-register", ResourceID: targetID, Revision: 0, CommandID: claim.CommandID}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	_, err = persistence.ApplyIdempotentTargetRegistration(ctx, pool, persistence.IdempotentTargetRegistration{
		TargetID: targetID, CommandID: claim.CommandID, CorrelationID: uuid.NewString(), OwnerToken: claim.OwnerToken,
		OutboxEventID: uuid.NewString(), DecisionInputID: uuid.NewString(), Registration: mustRegistration(t, "https://simulator.example"),
		Completion: persistence.IdempotencyCompletion{ActorID: claim.ActorID, Route: claim.Route, RawKey: claim.RawKey, ResponseStatus: 201, ResponseReference: reference},
	})
	if err != nil {
		t.Fatal(err)
	}
}
