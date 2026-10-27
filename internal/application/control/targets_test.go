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

func TestTargetServiceListsOnlyBoundedAuthorizedPages(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	store := &recordingTargetStore{page: TargetPage{Items: []TargetProjection{{TargetID: "10000000-0000-0000-0000-000000000001", State: "REGISTERED", Revision: 0, AsOf: now}}}}
	service := NewTargetService(store, func() time.Time { return now })
	viewer := access.Principal{Subject: "viewer", Roles: []access.Role{access.RoleViewer}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}
	page, err := service.ListTargets(context.Background(), viewer, "", 25)
	if err != nil || len(page.Items) != 1 || store.listCalls != 1 || store.pageSize != 25 {
		t.Fatalf("page=%+v calls=%d pageSize=%d err=%v", page, store.listCalls, store.pageSize, err)
	}
	if _, err := service.ListTargets(context.Background(), viewer, "", 101); err == nil || store.listCalls != 1 {
		t.Fatalf("unbounded page err=%v calls=%d", err, store.listCalls)
	}
	denied := viewer
	denied.Roles = []access.Role{"unknown"}
	if _, err := service.ListTargets(context.Background(), denied, "", 25); err == nil || store.listCalls != 1 {
		t.Fatalf("unauthorized list err=%v calls=%d", err, store.listCalls)
	}
}

type recordingTargetStore struct {
	calls     int
	listCalls int
	pageSize  int
	page      TargetPage
}

func (store *recordingTargetStore) RegisterTarget(_ context.Context, _ access.Principal, meta CommandMeta, targetID string, _ inventory.Registration) (TargetRegistrationResult, error) {
	store.calls++
	return TargetRegistrationResult{TargetID: targetID, CommandID: meta.CommandID, AcceptedAt: time.Unix(0, 0).UTC()}, nil
}

func (store *recordingTargetStore) ListTargets(_ context.Context, _ string, pageSize int) (TargetPage, error) {
	store.listCalls++
	store.pageSize = pageSize
	return store.page, nil
}
