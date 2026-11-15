//go:build postgresintegration

package postgres

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
	"github.com/parth10-05/fleetforge/internal/delivery/controlhttp"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestControlTargetRoutePersistsAtomicRegistration(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	service := control.NewTargetService(persistence.ControlTargetAdapter{Pool: pool}, func() time.Time { return now })
	verifier := integrationVerifier{principal: access.Principal{Subject: "user:admin", Roles: []access.Role{access.RoleAdmin}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}}
	router := controlhttp.NewRouterWithTargets(verifier, func() time.Time { return now }, service)
	request := httptest.NewRequest(http.MethodPost, "/v1/targets", strings.NewReader(`{"display_name":"rack-a","endpoint_reference":"https://simulator.example","target_key":"rack-a","credential_reference":"synthetic-test:rack-a","labels":{"site":"west","rack":"a"}}`))
	request.Header.Set("Authorization", "Bearer admin-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "target-http-registration")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope controlhttp.CommandEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	readRequest := httptest.NewRequest(http.MethodGet, "/v1/targets/"+envelope.Resource.Id.String(), nil)
	readRequest.Header.Set("Authorization", "Bearer admin-token")
	readRecorder := httptest.NewRecorder()
	router.ServeHTTP(readRecorder, readRequest)
	if readRecorder.Code != http.StatusOK || readRecorder.Header().Get("ETag") != "\"rev-0\"" {
		t.Fatalf("read status=%d etag=%q body=%s", readRecorder.Code, readRecorder.Header().Get("ETag"), readRecorder.Body.String())
	}
	var target controlhttp.Target
	if err := json.Unmarshal(readRecorder.Body.Bytes(), &target); err != nil || target.Id != envelope.Resource.Id || target.Revision != 0 {
		t.Fatalf("target=%+v err=%v", target, err)
	}
	// Map order differs but the semantic request and canonical idempotency hash do not.
	replayRequest := httptest.NewRequest(http.MethodPost, "/v1/targets", strings.NewReader(`{"display_name":"rack-a","endpoint_reference":"https://simulator.example","target_key":"rack-a","credential_reference":"synthetic-test:rack-a","labels":{"rack":"a","site":"west"}}`))
	replayRequest.Header.Set("Authorization", "Bearer admin-token")
	replayRequest.Header.Set("Content-Type", "application/json")
	replayRequest.Header.Set("Idempotency-Key", "target-http-registration")
	replayRecorder := httptest.NewRecorder()
	router.ServeHTTP(replayRecorder, replayRequest)
	if replayRecorder.Code != http.StatusCreated || replayRecorder.Body.String() != recorder.Body.String() {
		t.Fatalf("replay status=%d body=%s, want original=%s", replayRecorder.Code, replayRecorder.Body.String(), recorder.Body.String())
	}
	for _, table := range []string{"targets", "audit_events", "decision_inputs", "outbox_events"} {
		var count int
		query := "SELECT COUNT(*) FROM " + table
		if table != "targets" {
			query += " WHERE aggregate_id=$1::uuid"
			if err := pool.QueryRow(ctx, query, envelope.Resource.Id.String()).Scan(&count); err != nil || count != 1 {
				t.Fatalf("%s count=%d err=%v", table, count, err)
			}
			continue
		}
		if err := pool.QueryRow(ctx, query).Scan(&count); err != nil || count != 1 {
			t.Fatalf("targets count=%d err=%v", count, err)
		}
	}
}

func TestControlTargetListRouteUsesKeysetCursor(t *testing.T) {
	_, pool, _ := migratedHarness(t)
	now := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	service := control.NewTargetService(persistence.ControlTargetAdapter{Pool: pool}, func() time.Time { return now })
	verifier := integrationVerifier{principal: access.Principal{Subject: "user:admin", Roles: []access.Role{access.RoleAdmin}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}}
	router := controlhttp.NewRouterWithTargetsAndCursor(verifier, func() time.Time { return now }, service, []byte("abcdefghijklmnopqrstuvwxyz012345"))
	for _, name := range []string{"rack-a", "rack-b"} {
		request := httptest.NewRequest(http.MethodPost, "/v1/targets", strings.NewReader(`{"display_name":"`+name+`","endpoint_reference":"https://simulator.example","target_key":"`+name+`","credential_reference":"synthetic-test:`+name+`","labels":{"site":"west","rack":"`+name+`"}}`))
		request.Header.Set("Authorization", "Bearer admin-token")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "register-target-"+name)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("register %s status=%d body=%s", name, recorder.Code, recorder.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/targets?page_size=1", nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("first page status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var first controlhttp.TargetPage
	if err := json.Unmarshal(recorder.Body.Bytes(), &first); err != nil || len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatalf("first page=%+v err=%v", first, err)
	}
	request = httptest.NewRequest(http.MethodGet, "/v1/targets?page_size=1&cursor="+*first.NextCursor, nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("second page status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var second controlhttp.TargetPage
	if err := json.Unmarshal(recorder.Body.Bytes(), &second); err != nil || len(second.Items) != 1 || second.Items[0].Id == first.Items[0].Id || second.NextCursor != nil {
		t.Fatalf("second page=%+v err=%v", second, err)
	}
}

type integrationVerifier struct{ principal access.Principal }

func (verifier integrationVerifier) VerifyPrincipal(context.Context, string) (access.Principal, error) {
	return verifier.principal, nil
}
