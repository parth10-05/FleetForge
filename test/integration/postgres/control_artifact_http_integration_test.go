//go:build postgresintegration

package postgres

import (
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
	"github.com/parth10-05/fleetforge/internal/platform/localartifactissuer"
)

func TestControlArtifactInitiationRoutePersistsAtomicReplay(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	adapter := persistence.ControlArtifactAdapter{Pool: pool}
	service := control.NewArtifactService(adapter, func() time.Time { return now }).WithInitiation(adapter, localartifactissuer.New([]byte("01234567890123456789012345678901"), nil), "artifacts")
	verifier := integrationVerifier{principal: access.Principal{Subject: "user:admin", Roles: []access.Role{access.RoleAdmin}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}}
	router := controlhttp.NewRouterWithResourcesAndUpload(verifier, func() time.Time { return now }, nil, service, nil, "http://127.0.0.1:8090/v1/objects/upload")
	body := `{"sha256":"` + strings.Repeat("a", 64) + `","size_bytes":3,"media_type":"application/octet-stream","firmware_version":"v1"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/artifacts:initiate", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer admin-token")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "artifact-http-initiation")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d cache=%q body=%s", recorder.Code, recorder.Header().Get("Cache-Control"), recorder.Body.String())
	}
	var response controlhttp.ArtifactInitiationEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Upload.Grant == "" {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	replay := httptest.NewRequest(http.MethodPost, "/v1/artifacts:initiate", strings.NewReader(body))
	replay.Header.Set("Authorization", "Bearer admin-token")
	replay.Header.Set("Content-Type", "application/json")
	replay.Header.Set("Idempotency-Key", "artifact-http-initiation")
	replayRecorder := httptest.NewRecorder()
	router.ServeHTTP(replayRecorder, replay)
	if replayRecorder.Code != http.StatusCreated || replayRecorder.Body.String() != recorder.Body.String() {
		t.Fatalf("replay status=%d body=%s, want original=%s", replayRecorder.Code, replayRecorder.Body.String(), recorder.Body.String())
	}
	for _, table := range []string{"firmware_artifacts", "audit_events", "decision_inputs", "outbox_events"} {
		var count int
		query := "SELECT COUNT(*) FROM " + table
		if table != "firmware_artifacts" {
			query += " WHERE aggregate_id=$1::uuid"
		}
		if err := pool.QueryRow(ctx, query, response.Resource.Id.String()).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	var grantID, state string
	if err := pool.QueryRow(ctx, "SELECT upload_grant_id,state FROM firmware_artifacts WHERE id=$1::uuid", response.Resource.Id.String()).Scan(&grantID, &state); err != nil || grantID == "" || state != "UPLOADING" {
		t.Fatalf("grant=%q state=%q err=%v", grantID, state, err)
	}
}
