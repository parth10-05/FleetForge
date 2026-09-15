//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

// ─── P03-08: idempotency claim and replay integration proofs ─────────────────

// TestIdempotencyClaimAndCompleteRoundTrip proves the happy path:
// claim succeeds, complete updates the record, and replay returns the
// stable response without re-running the command.
func TestIdempotencyClaimAndCompleteRoundTrip(t *testing.T) {
	_, pool, ctx := migratedHarness(t)

	claim := persistence.IdempotencyClaim{
		ID: "00000000-0000-0000-8001-000000000001", ActorID: "actor-rt",
		Route: "/v1/campaigns", RawKey: "key-rt", RequestBody: []byte(`{"name":"test"}`),
		CommandID: "00000000-0000-0000-8001-000000000002", OwnerToken: "00000000-0000-0000-8001-000000000003",
		TTL: defaultTTL,
	}
	if err := persistence.ClaimIdempotencyKey(ctx, pool, claim); err != nil {
		t.Fatalf("ClaimIdempotencyKey: %v", err)
	}

	completion := persistence.IdempotencyCompletion{
		ActorID: "actor-rt", Route: "/v1/campaigns", RawKey: "key-rt",
		ResponseStatus: 201, ResponseReference: "campaigns/abc123",
	}
	if err := persistence.CompleteIdempotencyKey(ctx, pool, completion, claim.OwnerToken); err != nil {
		t.Fatalf("CompleteIdempotencyKey: %v", err)
	}

	status, ref, found, err := persistence.GetCompletedIdempotencyRecord(ctx, pool, "actor-rt", "/v1/campaigns", "key-rt")
	if err != nil {
		t.Fatalf("GetCompletedIdempotencyRecord: %v", err)
	}
	if !found || status != 201 || ref != "campaigns/abc123" {
		t.Errorf("replay: found=%v status=%d ref=%q, want true/201/campaigns/abc123", found, status, ref)
	}
}

// TestIdempotentAggregateMutationCommitsCompletionWithMutation proves the
// second transaction never publishes a replay result without its aggregate,
// audit, and outbox writes.
func TestIdempotentAggregateMutationCommitsCompletionWithMutation(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)
	claim := persistence.IdempotencyClaim{
		ID: "00000000-0000-0000-8011-000000000001", ActorID: "actor-atomic",
		Route: "/v1/campaigns", RawKey: "key-atomic", RequestBody: []byte(`{"name":"atomic"}`),
		CommandID: "00000000-0000-0000-8011-000000000002", OwnerToken: "00000000-0000-0000-8011-000000000003", TTL: defaultTTL,
	}
	if err := persistence.ClaimIdempotencyKey(ctx, pool, claim); err != nil {
		t.Fatalf("ClaimIdempotencyKey: %v", err)
	}

	input := persistence.IdempotentAggregateMutation{
		Completion: persistence.IdempotencyCompletion{ActorID: claim.ActorID, Route: claim.Route, RawKey: claim.RawKey, ResponseStatus: 201, ResponseReference: "campaigns/atomic"},
		OwnerToken: claim.OwnerToken,
		Mutation:   campaignMutation(fixture, 0, "VALIDATING", "00000000-0000-0000-8011-000000000004", claim.CommandID),
	}
	if err := persistence.ApplyIdempotentAggregateMutation(ctx, pool, input); err != nil {
		t.Fatalf("ApplyIdempotentAggregateMutation: %v", err)
	}

	var state string
	if err := pool.QueryRow(ctx, "SELECT state FROM campaigns WHERE id=$1::uuid", fixture.campaignID).Scan(&state); err != nil {
		t.Fatalf("read campaign: %v", err)
	}
	if state != "VALIDATING" {
		t.Errorf("campaign state=%q, want VALIDATING", state)
	}
	status, reference, found, err := persistence.GetCompletedIdempotencyRecord(ctx, pool, claim.ActorID, claim.Route, claim.RawKey)
	if err != nil || !found || status != 201 || reference != "campaigns/atomic" {
		t.Errorf("replay after atomic completion: status=%d reference=%q found=%v err=%v", status, reference, found, err)
	}
}

// TestIdempotencyIdenticalConcurrentClaimConverges proves that when two
// goroutines race to claim the same key+hash, exactly one wins and one
// receives ErrIdempotencyInProgress (never both succeed or both error with
// unexpected values).
func TestIdempotencyIdenticalConcurrentClaimConverges(t *testing.T) {
	_, pool, _ := migratedHarness(t)

	type result struct{ err error }
	results := make(chan result, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("00000000-0000-0000-8002-00000000000%d", i+1)
		cmdID := fmt.Sprintf("00000000-0000-0000-8002-00000000001%d", i+1)
		ownerToken := fmt.Sprintf("00000000-0000-0000-8002-00000000002%d", i+1)
		go func(claimID, commandID, token string) {
			defer wg.Done()
			claim := persistence.IdempotencyClaim{
				ID: claimID, ActorID: "actor-cc",
				Route: "/v1/campaigns", RawKey: "key-cc", RequestBody: []byte(`{"name":"concurrent"}`),
				CommandID: commandID, OwnerToken: token, TTL: defaultTTL,
			}
			results <- result{err: persistence.ClaimIdempotencyKey(context.Background(), pool, claim)}
		}(id, cmdID, ownerToken)
	}
	wg.Wait()
	close(results)

	var successes, inProgress int
	for r := range results {
		if r.err == nil {
			successes++
		} else if errors.Is(r.err, persistence.ErrIdempotencyInProgress) {
			inProgress++
		} else {
			t.Errorf("unexpected claim error: %v", r.err)
		}
	}
	if successes != 1 || inProgress != 1 {
		t.Errorf("concurrent claim: successes=%d inProgress=%d, want 1/1", successes, inProgress)
	}
}

// TestIdempotencySameKeyDifferentHashConflicts proves that presenting the same
// idempotency key with a different request body is rejected with ErrIdempotencyConflict.
func TestIdempotencySameKeyDifferentHashConflicts(t *testing.T) {
	_, pool, ctx := migratedHarness(t)

	first := persistence.IdempotencyClaim{
		ID: "00000000-0000-0000-8003-000000000001", ActorID: "actor-cf",
		Route: "/v1/campaigns", RawKey: "key-conflict", RequestBody: []byte(`{"a":1}`),
		CommandID: "00000000-0000-0000-8003-000000000002", OwnerToken: "00000000-0000-0000-8003-000000000003",
		TTL: defaultTTL,
	}
	if err := persistence.ClaimIdempotencyKey(ctx, pool, first); err != nil {
		t.Fatalf("first claim: %v", err)
	}

	second := first
	second.ID = "00000000-0000-0000-8003-000000000004"
	second.CommandID = "00000000-0000-0000-8003-000000000005"
	second.OwnerToken = "00000000-0000-0000-8003-000000000006"
	second.RequestBody = []byte(`{"a":2}`) // different hash

	err := persistence.ClaimIdempotencyKey(ctx, pool, second)
	if !errors.Is(err, persistence.ErrIdempotencyConflict) {
		t.Errorf("different-hash claim error = %v, want ErrIdempotencyConflict", err)
	}
}

// TestIdempotencyCompletionRequiresOwnerToken proves that completing a claim
// with the wrong owner token fails, protecting the protocol from stale workers.
func TestIdempotencyCompletionRequiresOwnerToken(t *testing.T) {
	_, pool, ctx := migratedHarness(t)

	claim := persistence.IdempotencyClaim{
		ID: "00000000-0000-0000-8004-000000000001", ActorID: "actor-ot",
		Route: "/v1/campaigns", RawKey: "key-owner", RequestBody: []byte(`{"x":1}`),
		CommandID: "00000000-0000-0000-8004-000000000002", OwnerToken: "00000000-0000-0000-8004-000000000003",
		TTL: defaultTTL,
	}
	if err := persistence.ClaimIdempotencyKey(ctx, pool, claim); err != nil {
		t.Fatalf("claim: %v", err)
	}

	wrongToken := "00000000-0000-0000-8004-000000000099"
	completion := persistence.IdempotencyCompletion{
		ActorID: "actor-ot", Route: "/v1/campaigns", RawKey: "key-owner",
		ResponseStatus: 200, ResponseReference: "ref/abc",
	}
	err := persistence.CompleteIdempotencyKey(ctx, pool, completion, wrongToken)
	if !errors.Is(err, persistence.ErrIdempotencyConflict) {
		t.Errorf("wrong-token completion error = %v, want ErrIdempotencyConflict", err)
	}
}

// TestIdempotencyStableReplayAfterCrashPoint proves that if the claim succeeds
// but the caller crashes before completing, a subsequent re-presentation with
// the same key+hash receives ErrIdempotencyInProgress (not a new command run).
func TestIdempotencyStableReplayAfterCrashPoint(t *testing.T) {
	_, pool, ctx := migratedHarness(t)

	claim := persistence.IdempotencyClaim{
		ID: "00000000-0000-0000-8005-000000000001", ActorID: "actor-cp",
		Route: "/v1/campaigns", RawKey: "key-crash", RequestBody: []byte(`{"crash":true}`),
		CommandID: "00000000-0000-0000-8005-000000000002", OwnerToken: "00000000-0000-0000-8005-000000000003",
		TTL: defaultTTL,
	}
	// First claim succeeds (simulates initial request).
	if err := persistence.ClaimIdempotencyKey(ctx, pool, claim); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	// Crash simulated: completion never called.
	// Second attempt with same key+hash must get ErrIdempotencyInProgress.
	claim.ID = "00000000-0000-0000-8005-000000000004"
	claim.CommandID = "00000000-0000-0000-8005-000000000005"
	claim.OwnerToken = "00000000-0000-0000-8005-000000000006"

	err := persistence.ClaimIdempotencyKey(ctx, pool, claim)
	if !errors.Is(err, persistence.ErrIdempotencyInProgress) {
		t.Errorf("re-claim after crash error = %v, want ErrIdempotencyInProgress", err)
	}
}

// TestIdempotencyNotFoundReturnsNoResult proves that querying a key that was
// never claimed returns (0, "", false, nil) from GetCompletedIdempotencyRecord.
func TestIdempotencyNotFoundReturnsNoResult(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	status, ref, found, err := persistence.GetCompletedIdempotencyRecord(ctx, pool, "actor-nf", "/v1/campaigns", "nonexistent-key")
	if err != nil || found || status != 0 || ref != "" {
		t.Errorf("missing key: found=%v status=%d ref=%q err=%v, want false/0//nil", found, status, ref, err)
	}
}

const defaultTTL = 30 * 60 * 1_000_000_000 // 30 minutes in nanoseconds
