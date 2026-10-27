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
)

func TestControlCampaignRoutePersistsImmutableDraftReplay(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	artifactID := "00000000-0000-0000-0000-000000000601"
	mustExecute(t, pool, ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,state) VALUES ('"+artifactID+"','v1','sha256:"+strings.Repeat("a", 64)+"','artifact.bin','VERIFIED')")
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	service := control.NewCampaignService(persistence.ControlCampaignAdapter{Pool: pool}, func() time.Time { return now })
	verifier := integrationVerifier{principal: access.Principal{Subject: "user:operator", Roles: []access.Role{access.RoleOperator}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}}
	router := controlhttp.NewRouterWithResourcesUploadAndCampaigns(verifier, func() time.Time { return now }, nil, nil, service, nil, "")
	body := `{"artifact_id":"` + artifactID + `","selector":{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"west"}]},"selection_seed":"seed","policy":{"version":"fleetforge.campaign-policy/v1","canary":{"unit":"absolute","value":1},"limits":{"global":1,"site":1,"rack":1},"failure_budget":{"numerator":1,"denominator":1},"expected_firmware_version":"v1","required_health_checks":["boot"],"maintenance":{"timezone":"UTC","weekdays":[1],"start_minute":1,"end_minute":2},"maximum_duration_seconds":60,"retry_budget":1,"observation_window_seconds":30,"observation_every_seconds":10,"observation_freshness_seconds":10,"governance_id":"change-1","strict_worst_case_capacity":false}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/campaigns", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer operator")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "campaign-create-0001")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response controlhttp.CommandEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	replay := httptest.NewRequest(http.MethodPost, "/v1/campaigns", strings.NewReader(body))
	replay.Header.Set("Authorization", "Bearer operator")
	replay.Header.Set("Content-Type", "application/json")
	replay.Header.Set("Idempotency-Key", "campaign-create-0001")
	replayRecorder := httptest.NewRecorder()
	router.ServeHTTP(replayRecorder, replay)
	if replayRecorder.Code != http.StatusCreated || replayRecorder.Body.String() != recorder.Body.String() {
		t.Fatalf("replay status=%d body=%s want=%s", replayRecorder.Code, replayRecorder.Body.String(), recorder.Body.String())
	}
	for _, table := range []string{"campaigns", "campaign_drafts", "audit_events", "decision_inputs", "outbox_events"} {
		var count int
		query := "SELECT COUNT(*) FROM " + table
		if table != "campaigns" && table != "campaign_drafts" {
			query += " WHERE aggregate_id=$1::uuid"
		} else {
			query += " WHERE " + map[bool]string{true: "campaign_id", false: "id"}[table == "campaign_drafts"] + "=$1::uuid"
		}
		if err := pool.QueryRow(ctx, query, response.Resource.Id.String()).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	var selector, policy string
	if err := pool.QueryRow(ctx, "SELECT canonical_selector::text,policy_json::text FROM campaign_drafts WHERE campaign_id=$1::uuid", response.Resource.Id.String()).Scan(&selector, &policy); err != nil || !strings.Contains(selector, "fleetforge.selector/v1") || !strings.Contains(policy, "fleetforge.campaign-policy/v1") {
		t.Fatalf("selector=%s policy=%s err=%v", selector, policy, err)
	}
}
