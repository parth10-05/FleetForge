package controlhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
)

func TestRegisterTargetRouteRequiresAdminAndWritesCommandEnvelope(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	registrar := &recordingTargetRegistrar{acceptedAt: now}
	verifier := tokenVerifierFunc(func(_ context.Context, token string) (access.Principal, error) {
		role := access.RoleViewer
		if token == "admin" {
			role = access.RoleAdmin
		}
		return access.Principal{Subject: "user", Roles: []access.Role{role}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}, nil
	})
	router := NewRouterWithTargets(verifier, func() time.Time { return now }, registrar)
	body := `{"display_name":"rack-a","endpoint_reference":"https://simulator.example","target_key":"rack-a","credential_reference":"synthetic-test:rack-a","labels":{"site":"west","rack":"a"}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/targets", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(idempotencyHeader, "target-register-request")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || registrar.calls != 1 || registrar.meta.Route != "/v1/targets" || len(registrar.meta.CanonicalRequest) == 0 {
		t.Fatalf("status=%d calls=%d meta=%+v body=%s", recorder.Code, registrar.calls, registrar.meta, recorder.Body.String())
	}
	var envelope CommandEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil || envelope.CommandId.String() != registrar.meta.CommandID || envelope.Resource.Id.String() != registrar.targetID {
		t.Fatalf("envelope=%+v err=%v", envelope, err)
	}
	denied := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/targets", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer viewer")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(idempotencyHeader, "target-register-request")
	router.ServeHTTP(denied, request)
	if denied.Code != http.StatusForbidden || registrar.calls != 1 {
		t.Fatalf("viewer status=%d calls=%d", denied.Code, registrar.calls)
	}
}

type recordingTargetRegistrar struct {
	calls      int
	meta       control.CommandMeta
	targetID   string
	acceptedAt time.Time
}

func (registrar *recordingTargetRegistrar) RegisterTarget(_ context.Context, _ access.Principal, meta control.CommandMeta, command control.RegisterTargetCommand) (control.TargetRegistrationResult, error) {
	registrar.calls++
	registrar.meta, registrar.targetID = meta, command.TargetID
	return control.TargetRegistrationResult{TargetID: command.TargetID, CommandID: meta.CommandID, AcceptedAt: registrar.acceptedAt}, nil
}
