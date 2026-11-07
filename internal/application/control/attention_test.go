package control

import (
	"context"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
)

type attentionStore struct{ calls int }

func (store *attentionStore) ResolveAttention(context.Context, access.Principal, CommandMeta, string, int64, AttentionResolutionCommand) (AttentionResolutionResult, error) {
	store.calls++
	return AttentionResolutionResult{}, nil
}

func TestAttentionServiceRequiresRecentAdmin(t *testing.T) {
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	store := &attentionStore{}
	service := NewAttentionService(store, func() time.Time { return now })
	meta := CommandMeta{ClaimID: "claim", CommandID: "command", CorrelationID: "correlation", OwnerToken: "owner", Route: "/v1/target-updates/{target_update_id}:resolve-attention", IdempotencyKey: "0123456789abcdef", CanonicalRequest: []byte(`{}`)}
	command := AttentionResolutionCommand{Action: "ADOPT", ReasonCode: "operator.reviewed", Reason: "verified"}
	principal := access.Principal{Subject: "admin", Roles: []access.Role{access.RoleAdmin}, IssuerClass: "local-test", AuthenticatedAt: now.Add(-16 * time.Minute), ExpiresAt: now.Add(time.Minute)}
	if _, err := service.ResolveAttention(context.Background(), principal, meta, "update", 3, command); err == nil || store.calls != 0 {
		t.Fatalf("stale principal err=%v calls=%d", err, store.calls)
	}
	principal.AuthenticatedAt = now.Add(-time.Minute)
	if _, err := service.ResolveAttention(context.Background(), principal, meta, "update", 3, command); err != nil || store.calls != 1 {
		t.Fatalf("authorized principal err=%v calls=%d", err, store.calls)
	}
}
