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
