//go:build postgresintegration

package postgres

import (
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestControlTargetAdapterReplaysCompletedRegistration(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	service := control.NewTargetService(persistence.ControlTargetAdapter{Pool: pool}, func() time.Time { return now })
	principal := access.Principal{Subject: "user:admin", Roles: []access.Role{access.RoleAdmin}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}
	meta := control.CommandMeta{ClaimID: "00000000-0000-0000-2805-000000000001", CommandID: "00000000-0000-0000-2805-000000000002", CorrelationID: "00000000-0000-0000-2805-000000000003", OwnerToken: "00000000-0000-0000-2805-000000000004", OutboxEventID: "00000000-0000-0000-2805-000000000005", DecisionInputID: "00000000-0000-0000-2805-000000000006", Route: "/v1/targets", IdempotencyKey: "target-adapter-request", CanonicalRequest: []byte(`{"target":"rack-a"}`)}
	command := control.RegisterTargetCommand{TargetID: "00000000-0000-0000-2805-000000000007", ExternalID: "rack-a", Origin: "https://simulator.example", TargetKey: "rack-a", CredentialReference: "synthetic-test:rack-a", Labels: map[string]string{"site": "west", "rack": "a"}}
	first, err := service.RegisterTarget(ctx, principal, meta, command)
	if err != nil || first.TargetID != command.TargetID || first.Revision != 0 {
		t.Fatalf("first result=%+v err=%v", first, err)
	}
	meta.ClaimID = "00000000-0000-0000-2805-000000000008"
	meta.OwnerToken = "00000000-0000-0000-2805-000000000009"
	meta.OutboxEventID = "00000000-0000-0000-2805-000000000010"
	meta.DecisionInputID = "00000000-0000-0000-2805-000000000011"
	replay, err := service.RegisterTarget(ctx, principal, meta, command)
	if err != nil || replay != first {
		t.Fatalf("replay result=%+v first=%+v err=%v", replay, first, err)
	}
}

func TestControlTargetAdapterQueuesDiscoveryAtomicallyAndReplays(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	service := control.NewTargetService(persistence.ControlTargetAdapter{Pool: pool}, func() time.Time { return now })
	principal := access.Principal{Subject: "user:admin", Roles: []access.Role{access.RoleAdmin}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}
	targetID := "00000000-0000-0000-2806-000000000001"
	registration := control.CommandMeta{ClaimID: "00000000-0000-0000-2806-000000000002", CommandID: "00000000-0000-0000-2806-000000000003", CorrelationID: "00000000-0000-0000-2806-000000000004", OwnerToken: "00000000-0000-0000-2806-000000000005", OutboxEventID: "00000000-0000-0000-2806-000000000006", DecisionInputID: "00000000-0000-0000-2806-000000000007", Route: "/v1/targets", IdempotencyKey: "target-registration-request", CanonicalRequest: []byte(`{"target":"rack-a"}`)}
	if _, err := service.RegisterTarget(ctx, principal, registration, control.RegisterTargetCommand{TargetID: targetID, ExternalID: "rack-a", Origin: "https://simulator.example", TargetKey: "rack-a", CredentialReference: "synthetic-test:rack-a", Labels: map[string]string{"site": "west", "rack": "a"}}); err != nil {
		t.Fatal(err)
	}
	meta := control.CommandMeta{ClaimID: "00000000-0000-0000-2806-000000000008", CommandID: "00000000-0000-0000-2806-000000000009", CorrelationID: "00000000-0000-0000-2806-000000000010", OwnerToken: "00000000-0000-0000-2806-000000000011", OutboxEventID: "00000000-0000-0000-2806-000000000012", DecisionInputID: "00000000-0000-0000-2806-000000000013", WorkID: "00000000-0000-0000-2806-000000000014", Route: "/v1/targets/{target_id}:discover", IdempotencyKey: "target-discovery-request", CanonicalRequest: []byte(`{"if_match":"\"rev-0\"","target_id":"00000000-0000-0000-2806-000000000001"}`)}
	first, err := service.DiscoverTarget(ctx, principal, meta, targetID, 0)
	if err != nil || first.TargetID != targetID || first.Revision != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	for _, assertion := range []struct {
		table, column string
		want          int
	}{
		{table: "audit_events", column: "aggregate_id", want: 2},
		{table: "decision_inputs", column: "aggregate_id", want: 2},
		{table: "outbox_events", column: "aggregate_id", want: 3},
		{table: "due_work", column: "subject_id", want: 1},
	} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+assertion.table+" WHERE "+assertion.column+"=$1::uuid", targetID).Scan(&count); err != nil || count != assertion.want {
			t.Fatalf("%s count=%d want=%d err=%v", assertion.table, count, assertion.want, err)
		}
	}
	meta.ClaimID = "00000000-0000-0000-2806-000000000015"
	meta.OwnerToken = "00000000-0000-0000-2806-000000000016"
	meta.OutboxEventID = "00000000-0000-0000-2806-000000000017"
	meta.DecisionInputID = "00000000-0000-0000-2806-000000000018"
	meta.WorkID = "00000000-0000-0000-2806-000000000019"
	replay, err := service.DiscoverTarget(ctx, principal, meta, targetID, 0)
	if err != nil || replay != first {
		t.Fatalf("replay=%+v first=%+v err=%v", replay, first, err)
	}
}
