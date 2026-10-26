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
	replayRequest := httptest.NewRequest(http.MethodPost, "/v1/targets", strings.NewReader(`{"display_name":"rack-a","endpoint_reference":"https://simulator.example","target_key":"rack-a","credential_reference":"synthetic-test:rack-a","labels":{"site":"west","rack":"a"}}`))
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

type integrationVerifier struct{ principal access.Principal }

func (verifier integrationVerifier) VerifyPrincipal(context.Context, string) (access.Principal, error) {
	return verifier.principal, nil
}
