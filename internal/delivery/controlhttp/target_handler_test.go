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

func TestGetTargetRouteAllowsViewerWithoutExistenceLeak(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	targetID := "10000000-0000-0000-0000-000000000001"
	registrar := &recordingTargetRegistrar{acceptedAt: now, targetID: targetID, result: control.TargetRegistrationResult{TargetID: targetID, Revision: 0, AcceptedAt: now}, state: "REGISTERED"}
	verifier := tokenVerifierFunc(func(_ context.Context, token string) (access.Principal, error) {
		role := access.RoleViewer
		if token == "none" {
			role = "unknown"
		}
		return access.Principal{Subject: "user", Roles: []access.Role{role}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}, nil
	})
	router := NewRouterWithTargets(verifier, func() time.Time { return now }, registrar)
	request := httptest.NewRequest(http.MethodGet, "/v1/targets/"+targetID, nil)
	request.Header.Set("Authorization", "Bearer viewer")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("ETag") != "\"rev-0\"" || registrar.readCalls != 1 {
		t.Fatalf("status=%d etag=%q calls=%d body=%s", recorder.Code, recorder.Header().Get("ETag"), registrar.readCalls, recorder.Body.String())
	}
	denied := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/targets/"+targetID, nil)
	request.Header.Set("Authorization", "Bearer none")
	router.ServeHTTP(denied, request)
	if denied.Code != http.StatusForbidden || registrar.readCalls != 1 {
		t.Fatalf("unknown role status=%d calls=%d", denied.Code, registrar.readCalls)
	}
	notFound := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/targets/10000000-0000-0000-0000-000000000002", nil)
	request.Header.Set("Authorization", "Bearer viewer")
	router.ServeHTTP(notFound, request)
	if notFound.Code != http.StatusNotFound || registrar.readCalls != 2 {
		t.Fatalf("not found status=%d calls=%d", notFound.Code, registrar.readCalls)
	}
}

func TestListTargetsRouteUsesBoundedSignedCursor(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	first := "10000000-0000-0000-0000-000000000001"
	second := "10000000-0000-0000-0000-000000000002"
	registrar := &recordingTargetRegistrar{page: control.TargetPage{Items: []control.TargetProjection{{TargetID: first, Revision: 0, AsOf: now, State: "REGISTERED"}}, NextTargetID: second}}
	verifier := tokenVerifierFunc(func(_ context.Context, _ string) (access.Principal, error) {
		return access.Principal{Subject: "viewer", Roles: []access.Role{access.RoleViewer}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}, nil
	})
	key := []byte("abcdefghijklmnopqrstuvwxyz012345")
	router := NewRouterWithTargetsAndCursor(verifier, func() time.Time { return now }, registrar, key)
	request := httptest.NewRequest(http.MethodGet, "/v1/targets?page_size=1", nil)
	request.Header.Set("Authorization", "Bearer viewer")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || registrar.listCalls != 1 || registrar.pageSize != 1 || registrar.afterID != "" {
		t.Fatalf("status=%d calls=%d pageSize=%d after=%q body=%s", recorder.Code, registrar.listCalls, registrar.pageSize, registrar.afterID, recorder.Body.String())
	}
	var page TargetPage
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/targets?page_size=1&cursor="+*page.NextCursor, nil)
	request.Header.Set("Authorization", "Bearer viewer")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || registrar.listCalls != 2 || registrar.afterID != second {
		t.Fatalf("status=%d calls=%d after=%q body=%s", recorder.Code, registrar.listCalls, registrar.afterID, recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/targets?page_size=2&cursor="+*page.NextCursor, nil)
	request.Header.Set("Authorization", "Bearer viewer")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || registrar.listCalls != 2 {
		t.Fatalf("mismatched cursor status=%d calls=%d", recorder.Code, registrar.listCalls)
	}
}

func TestDiscoverTargetRouteRequiresPreconditionAndAdmin(t *testing.T) {
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	targetID := "10000000-0000-0000-0000-000000000001"
	registrar := &recordingTargetRegistrar{targetID: targetID, discovery: control.TargetRegistrationResult{TargetID: targetID, CommandID: "10000000-0000-0000-0000-000000000002", Revision: 1, AcceptedAt: now}}
	verifier := tokenVerifierFunc(func(_ context.Context, token string) (access.Principal, error) {
		role := access.RoleViewer
		if token == "admin" {
			role = access.RoleAdmin
		}
		return access.Principal{Subject: "user", Roles: []access.Role{role}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}, nil
	})
	router := NewRouterWithTargets(verifier, func() time.Time { return now }, registrar)
	request := httptest.NewRequest(http.MethodPost, "/v1/targets/"+targetID+":discover", nil)
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set(idempotencyHeader, "target-discovery-request")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusPreconditionRequired || registrar.discoveryCalls != 0 {
		t.Fatalf("missing precondition status=%d calls=%d", recorder.Code, registrar.discoveryCalls)
	}
	request.Header.Set("If-Match", "\"rev-0\"")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted || registrar.discoveryCalls != 1 {
		t.Fatalf("accepted status=%d calls=%d body=%s", recorder.Code, registrar.discoveryCalls, recorder.Body.String())
	}
	denied := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/targets/"+targetID+":discover", nil)
	request.Header.Set("Authorization", "Bearer viewer")
	request.Header.Set(idempotencyHeader, "target-discovery-request")
	request.Header.Set("If-Match", "\"rev-0\"")
	router.ServeHTTP(denied, request)
	if denied.Code != http.StatusForbidden || registrar.discoveryCalls != 1 {
		t.Fatalf("viewer status=%d calls=%d", denied.Code, registrar.discoveryCalls)
	}
}

type recordingTargetRegistrar struct {
	calls          int
	meta           control.CommandMeta
	targetID       string
	acceptedAt     time.Time
	readCalls      int
	result         control.TargetRegistrationResult
	state          string
	discovery      control.TargetRegistrationResult
	discoveryCalls int
	page           control.TargetPage
	listCalls      int
	afterID        string
	pageSize       int
}

func (registrar *recordingTargetRegistrar) ListTargets(_ context.Context, _ access.Principal, afterID string, pageSize int) (control.TargetPage, error) {
	registrar.listCalls++
	registrar.afterID, registrar.pageSize = afterID, pageSize
	return registrar.page, nil
}

func (registrar *recordingTargetRegistrar) DiscoverTarget(_ context.Context, _ access.Principal, _ control.CommandMeta, targetID string, revision int64) (control.TargetRegistrationResult, error) {
	registrar.discoveryCalls++
	if targetID != registrar.targetID || revision != 0 {
		return control.TargetRegistrationResult{}, control.ErrTargetRevisionConflict
	}
	return registrar.discovery, nil
}

func (registrar *recordingTargetRegistrar) GetTarget(_ context.Context, _ access.Principal, targetID string) (control.TargetRegistrationResult, string, error) {
	registrar.readCalls++
	if targetID != registrar.targetID {
		return control.TargetRegistrationResult{}, "", control.ErrTargetNotFound
	}
	return registrar.result, registrar.state, nil
}

func (registrar *recordingTargetRegistrar) RegisterTarget(_ context.Context, _ access.Principal, meta control.CommandMeta, command control.RegisterTargetCommand) (control.TargetRegistrationResult, error) {
	registrar.calls++
	registrar.meta, registrar.targetID = meta, command.TargetID
	return control.TargetRegistrationResult{TargetID: command.TargetID, CommandID: meta.CommandID, AcceptedAt: registrar.acceptedAt}, nil
}
