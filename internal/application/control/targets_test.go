package control

import (
	"context"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
)

func TestTargetServiceAuthorizesAndValidatesBeforeStore(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	store := &recordingTargetStore{}
	service := NewTargetService(store, func() time.Time { return now })
	principal := access.Principal{Subject: "admin", Roles: []access.Role{access.RoleAdmin}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}
	meta := CommandMeta{ClaimID: "claim", CommandID: "command", CorrelationID: "correlation", OwnerToken: "owner", OutboxEventID: "outbox", DecisionInputID: "decision", Route: "/v1/targets", IdempotencyKey: "key", CanonicalRequest: []byte(`{}`)}
	result, err := service.RegisterTarget(context.Background(), principal, meta, RegisterTargetCommand{TargetID: "target", ExternalID: "rack-a", Origin: "https://simulator.example", TargetKey: "rack-a", CredentialReference: "synthetic-test:rack-a", Labels: map[string]string{"site": "west", "rack": "a"}})
	if err != nil || result.TargetID != "target" || store.calls != 1 {
		t.Fatalf("result=%+v calls=%d err=%v", result, store.calls, err)
	}
	viewer := principal
	viewer.Roles = []access.Role{access.RoleViewer}
	if _, err := service.RegisterTarget(context.Background(), viewer, meta, RegisterTargetCommand{TargetID: "other"}); err == nil || store.calls != 1 {
		t.Fatalf("unauthorized registration err=%v calls=%d", err, store.calls)
	}
}

type recordingTargetStore struct{ calls int }

func (store *recordingTargetStore) RegisterTarget(_ context.Context, _ access.Principal, meta CommandMeta, targetID string, _ inventory.Registration) (TargetRegistrationResult, error) {
	store.calls++
	return TargetRegistrationResult{TargetID: targetID, CommandID: meta.CommandID, AcceptedAt: time.Unix(0, 0).UTC()}, nil
}
