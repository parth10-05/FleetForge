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

func TestValidateCampaignHandlerRequiresRevisionAndQueuesWork(t *testing.T) {
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	campaignID := "10000000-0000-0000-0000-000000000001"
	validator := validateCampaignFunc(func(_ context.Context, _ access.Principal, meta control.CommandMeta, id string, revision int64) (control.CampaignCreationResult, error) {
		if id != campaignID || revision != 3 || meta.Route != "/v1/campaigns/{campaign_id}:validate" || meta.WorkID == "" {
			t.Fatalf("unexpected validation command: id=%q revision=%d meta=%+v", id, revision, meta)
		}
		return control.CampaignCreationResult{CampaignID: id, CommandID: meta.CommandID, Revision: 4, AcceptedAt: now}, nil
	})
	handler := validateCampaignHandler(validator)
	request := httptest.NewRequest(http.MethodPost, "/v1/campaigns/"+campaignID+":validate", nil)
	request.SetPathValue("campaign_id", campaignID)
	request.Header.Set(idempotencyHeader, "campaign-validate-1")
	request.Header.Set("If-Match", "\"rev-3\"")
	request = request.WithContext(context.WithValue(request.Context(), principalKey{}, access.Principal{Subject: "operator", Roles: []access.Role{access.RoleOperator}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	missing := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/campaigns/"+campaignID+":validate", nil)
	request.SetPathValue("campaign_id", campaignID)
	request.Header.Set(idempotencyHeader, "campaign-validate-1")
	handler.ServeHTTP(missing, request)
	if missing.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing If-Match status=%d", missing.Code)
	}
}

type validateCampaignFunc func(context.Context, access.Principal, control.CommandMeta, string, int64) (control.CampaignCreationResult, error)

func (f validateCampaignFunc) ValidateCampaign(ctx context.Context, principal access.Principal, meta control.CommandMeta, campaignID string, revision int64) (control.CampaignCreationResult, error) {
	return f(ctx, principal, meta, campaignID, revision)
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
