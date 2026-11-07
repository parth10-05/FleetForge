package controlhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
)

type attentionVerifierFunc func(context.Context, string) (access.Principal, error)

func (fn attentionVerifierFunc) VerifyPrincipal(ctx context.Context, token string) (access.Principal, error) {
	return fn(ctx, token)
}

func TestAttentionRouteRequiresPreconditionsAndForwardsProof(t *testing.T) {
	now := time.Now().UTC()
	verifier := attentionVerifierFunc(func(_ context.Context, _ string) (access.Principal, error) {
		return access.Principal{Subject: "admin", Roles: []access.Role{access.RoleAdmin}, IssuerClass: "test", AuthenticatedAt: now, ExpiresAt: now.Add(time.Minute)}, nil
	})
	called := false
	resolver := attentionResolverFunc(func(_ context.Context, _ access.Principal, _ control.CommandMeta, id string, revision int64, command control.AttentionResolutionCommand) (control.AttentionResolutionResult, error) {
		called = id == "00000000-0000-0000-0000-000000000001" && revision == 3 && command.Action == "TERMINAL" && command.ReservationDisposition == "QUARANTINE"
		return control.AttentionResolutionResult{TargetUpdateID: id, CommandID: "00000000-0000-0000-0000-000000000002", Revision: 4, AcceptedAt: now}, nil
	})
	router := NewRouterWithResourcesUploadAndCampaigns(verifier, func() time.Time { return now }, nil, nil, nil, nil, "", resolver)
	req := httptest.NewRequest(http.MethodPost, "/v1/target-updates/00000000-0000-0000-0000-000000000001:resolve-attention", strings.NewReader(`{"action":"TERMINAL","reason_code":"operator.reviewed","reason":"verified","reservation_disposition":"QUARANTINE"}`))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Idempotency-Key", "0123456789abcdef")
	req.Header.Set("If-Match", `"rev-3"`)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusAccepted || !called {
		t.Fatalf("status=%d called=%t body=%s", response.Code, called, response.Body.String())
	}
}
