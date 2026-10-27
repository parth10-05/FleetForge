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

func TestCreateCampaignRouteRequiresOperatorAndCanonicalizesDraft(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	creator := &recordingCampaignCreator{}
	verifier := tokenVerifierFunc(func(_ context.Context, token string) (access.Principal, error) {
		role := access.RoleViewer
		if token == "operator" {
			role = access.RoleOperator
		}
		return access.Principal{Subject: "user", Roles: []access.Role{role}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}, nil
	})
	router := NewRouterWithResourcesUploadAndCampaigns(verifier, func() time.Time { return now }, nil, nil, creator, nil, "")
	body := `{"artifact_id":"10000000-0000-0000-0000-000000000001","selector":{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"west"}]},"selection_seed":"seed","policy":{"version":"fleetforge.campaign-policy/v1","canary":{"unit":"absolute","value":1},"limits":{"global":1,"site":1,"rack":1},"failure_budget":{"numerator":1,"denominator":1},"expected_firmware_version":"v1","required_health_checks":["boot"],"maintenance":{"timezone":"UTC","weekdays":[1],"start_minute":1,"end_minute":2},"maximum_duration_seconds":60,"retry_budget":1,"observation_window_seconds":30,"observation_every_seconds":10,"observation_freshness_seconds":10,"governance_id":"change-1","strict_worst_case_capacity":false}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/campaigns", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer operator")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(idempotencyHeader, "campaign-create-1")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || creator.calls != 1 || len(creator.command.Selector.CanonicalJSON()) == 0 {
		t.Fatalf("status=%d calls=%d body=%s", recorder.Code, creator.calls, recorder.Body.String())
	}
	denied := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/campaigns", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer viewer")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(idempotencyHeader, "campaign-create-1")
	router.ServeHTTP(denied, request)
	if denied.Code != http.StatusForbidden || creator.calls != 1 {
		t.Fatalf("status=%d calls=%d", denied.Code, creator.calls)
	}
	invalid := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/campaigns", strings.NewReader(strings.Replace(body, `"strict_worst_case_capacity":false`, `"strict_worst_case_capacity":false,"unknown":true`, 1)))
	request.Header.Set("Authorization", "Bearer operator")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(idempotencyHeader, "campaign-create-0002")
	router.ServeHTTP(invalid, request)
	if invalid.Code != http.StatusBadRequest || creator.calls != 1 {
		t.Fatalf("unknown policy status=%d calls=%d", invalid.Code, creator.calls)
	}
}

type recordingCampaignCreator struct {
	calls   int
	command control.CreateCampaignCommand
}

func (creator *recordingCampaignCreator) CreateCampaign(_ context.Context, _ access.Principal, meta control.CommandMeta, command control.CreateCampaignCommand) (control.CampaignCreationResult, error) {
	creator.calls++
	creator.command = command
	return control.CampaignCreationResult{CampaignID: command.CampaignID, CommandID: meta.CommandID, AcceptedAt: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)}, nil
}
