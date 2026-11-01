package controlhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
	"github.com/parth10-05/fleetforge/internal/domain/campaignplan"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
)

type campaignCreator interface {
	CreateCampaign(context.Context, access.Principal, control.CommandMeta, control.CreateCampaignCommand) (control.CampaignCreationResult, error)
}

type campaignValidator interface {
	ValidateCampaign(context.Context, access.Principal, control.CommandMeta, string, int64) (control.CampaignCreationResult, error)
}

func createCampaignHandler(creator campaignCreator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, err := idempotencyKey(r.Header.Get(idempotencyHeader))
		if err != nil {
			writeControlProblem(w, r, http.StatusBadRequest, "invalid_idempotency_key")
			return
		}
		var input CampaignCreation
		if err := decodeJSON(r, &input); err != nil {
			writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
			return
		}
		selectorRaw, _ := json.Marshal(input.Selector)
		selector, err := inventory.ParseSelector(selectorRaw)
		if err != nil {
			writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
			return
		}
		policyRaw, _ := json.Marshal(input.Policy)
		var policy campaignplan.Policy
		decoder := json.NewDecoder(bytes.NewReader(policyRaw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&policy) != nil || decoder.More() {
			writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
			return
		}
		canonical, err := json.Marshal(input)
		if err != nil {
			writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
			return
		}
		principal, ok := principalFromContext(r.Context())
		if !ok {
			writeUnauthorized(w, correlationID(r.Context()))
			return
		}
		result, err := creator.CreateCampaign(r.Context(), principal, control.CommandMeta{ClaimID: uuid.NewString(), CommandID: uuid.NewString(), CorrelationID: correlationID(r.Context()), OwnerToken: uuid.NewString(), OutboxEventID: uuid.NewString(), DecisionInputID: uuid.NewString(), Route: "/v1/campaigns", IdempotencyKey: key, CanonicalRequest: canonical}, control.CreateCampaignCommand{CampaignID: uuid.NewString(), PolicyVersionID: uuid.NewString(), ArtifactID: input.ArtifactId.String(), SelectionSeed: input.SelectionSeed, Selector: selector, Policy: policy})
		if err != nil {
			if errors.Is(err, control.ErrInvalidCampaignCreation) {
				writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
				return
			}
			writeControlProblem(w, r, http.StatusConflict, "command_conflict")
			return
		}
		id, err := uuid.Parse(result.CampaignID)
		if err != nil {
			writeControlProblem(w, r, http.StatusInternalServerError, "internal_error")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(CommandEnvelope{CommandId: uuid.MustParse(result.CommandID), Resource: ResourceReference{Kind: "campaign", Id: id, Href: "/v1/campaigns/" + result.CampaignID}, AcceptedAt: result.AcceptedAt})
	})
}

func validateCampaignHandler(validator campaignValidator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, err := idempotencyKey(r.Header.Get(idempotencyHeader))
		if err != nil {
			writeControlProblem(w, r, http.StatusBadRequest, "invalid_idempotency_key")
			return
		}
		if r.Header.Get("If-Match") == "" {
			writeControlProblem(w, r, http.StatusPreconditionRequired, "precondition_required")
			return
		}
		revision, err := parseRevisionETag(r.Header.Get("If-Match"))
		if err != nil {
			writeControlProblem(w, r, http.StatusBadRequest, "invalid_precondition")
			return
		}
		principal, ok := principalFromContext(r.Context())
		if !ok {
			writeUnauthorized(w, correlationID(r.Context()))
			return
		}
		campaignID := r.PathValue("campaign_id")
		if _, err := uuid.Parse(campaignID); err != nil {
			writeControlProblem(w, r, http.StatusNotFound, "not_found")
			return
		}
		canonical, _ := json.Marshal(map[string]string{"campaign_id": campaignID, "if_match": r.Header.Get("If-Match")})
		result, err := validator.ValidateCampaign(r.Context(), principal, control.CommandMeta{ClaimID: uuid.NewString(), CommandID: uuid.NewString(), CorrelationID: correlationID(r.Context()), OwnerToken: uuid.NewString(), OutboxEventID: uuid.NewString(), DecisionInputID: uuid.NewString(), WorkID: uuid.NewString(), Route: "/v1/campaigns/{campaign_id}:validate", IdempotencyKey: key, CanonicalRequest: canonical}, campaignID, revision)
		if err != nil {
			writeControlProblem(w, r, http.StatusPreconditionFailed, "revision_conflict")
			return
		}
		id, err := uuid.Parse(result.CampaignID)
		if err != nil {
			writeControlProblem(w, r, http.StatusInternalServerError, "internal_error")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(CommandEnvelope{CommandId: uuid.MustParse(result.CommandID), Resource: ResourceReference{Kind: "campaign", Id: id, Href: "/v1/campaigns/" + result.CampaignID}, Revision: ptr(int(result.Revision)), AcceptedAt: result.AcceptedAt})
	})
}
