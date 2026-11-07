package controlhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

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

type campaignStarter interface {
	StartCampaign(context.Context, access.Principal, control.CommandMeta, string, int64) (control.CampaignCreationResult, error)
}

type campaignPauser interface {
	PauseCampaign(context.Context, access.Principal, control.CommandMeta, string, int64) (control.CampaignCreationResult, error)
}

type campaignReader interface {
	GetCampaign(context.Context, access.Principal, string) (control.CampaignProjection, error)
}

type campaignTargetLister interface {
	ListCampaignTargets(context.Context, access.Principal, string, string, int) (control.CampaignTargetPage, error)
}

type campaignAuditLister interface {
	ListCampaignAudit(context.Context, access.Principal, string, int64, int64, int) (control.CampaignAuditPage, error)
}

func validateCampaignRoute(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		campaignID, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/v1/campaigns/"), ":validate")
		if !ok || campaignID == "" || strings.Contains(campaignID, "/") {
			writeControlProblem(w, r, http.StatusNotFound, "not_found")
			return
		}
		r.SetPathValue("campaign_id", campaignID)
		handler.ServeHTTP(w, r)
	})
}

func campaignActionRoute(actions map[string]http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1/campaigns/")
		campaignID, action, ok := strings.Cut(path, ":")
		if !ok || campaignID == "" || strings.Contains(campaignID, "/") || actions[action] == nil {
			writeControlProblem(w, r, http.StatusNotFound, "not_found")
			return
		}
		r.SetPathValue("campaign_id", campaignID)
		actions[action].ServeHTTP(w, r)
	})
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

func getCampaignHandler(reader campaignReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		projection, err := reader.GetCampaign(r.Context(), principal, campaignID)
		if errors.Is(err, control.ErrCampaignNotFound) {
			writeControlProblem(w, r, http.StatusNotFound, "not_found")
			return
		}
		if err != nil {
			writeControlProblem(w, r, http.StatusInternalServerError, "internal_error")
			return
		}
		id, err := uuid.Parse(projection.CampaignID)
		if err != nil {
			writeControlProblem(w, r, http.StatusInternalServerError, "internal_error")
			return
		}
		etag, err := revisionETag(projection.Revision)
		if err != nil {
			writeControlProblem(w, r, http.StatusInternalServerError, "internal_error")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", etag)
		_ = json.NewEncoder(w).Encode(Campaign{Id: id, Revision: int(projection.Revision), AsOf: projection.AsOf, State: projection.State})
	})
}

func listCampaignTargetsHandler(lister campaignTargetLister, cursorKey []byte, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for key, values := range query {
			if (key != "page_size" && key != "cursor") || len(values) != 1 || values[0] == "" {
				writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
				return
			}
		}
		pageSize := 50
		if raw := query.Get("page_size"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 100 || strconv.Itoa(value) != raw {
				writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
				return
			}
			pageSize = value
		}
		campaignID := r.PathValue("campaign_id")
		if _, err := uuid.Parse(campaignID); err != nil {
			writeControlProblem(w, r, http.StatusNotFound, "not_found")
			return
		}
		afterID := ""
		if cursor := query.Get("cursor"); cursor != "" {
			var err error
			afterID, err = decodeCampaignTargetCursor(cursorKey, cursor, campaignID, pageSize, now())
			if err != nil {
				writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
				return
			}
		}
		principal, ok := principalFromContext(r.Context())
		if !ok {
			writeUnauthorized(w, correlationID(r.Context()))
			return
		}
		page, err := lister.ListCampaignTargets(r.Context(), principal, campaignID, afterID, pageSize)
		if errors.Is(err, control.ErrCampaignNotFound) {
			writeControlProblem(w, r, http.StatusNotFound, "not_found")
			return
		}
		if err != nil {
			writeControlProblem(w, r, http.StatusInternalServerError, "internal_error")
			return
		}
		items := make([]Target, 0, len(page.Items))
		for _, item := range page.Items {
			id, err := uuid.Parse(item.TargetID)
			if err != nil {
				writeControlProblem(w, r, http.StatusInternalServerError, "internal_error")
				return
			}
			items = append(items, Target{Id: id, Revision: int(item.Revision), AsOf: item.AsOf, State: item.State})
		}
		response := TargetPage{Items: items}
		if page.NextTargetID != "" {
			next, err := encodeCampaignTargetCursor(cursorKey, campaignID, page.NextTargetID, pageSize, now())
			if err != nil {
				writeControlProblem(w, r, http.StatusInternalServerError, "internal_error")
				return
			}
			response.NextCursor = &next
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
}

func listCampaignAuditHandler(lister campaignAuditLister, cursorKey []byte, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for key, values := range query {
			if (key != "page_size" && key != "cursor") || len(values) != 1 || values[0] == "" {
				writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
				return
			}
		}
		pageSize := 50
		if raw := query.Get("page_size"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 100 || strconv.Itoa(value) != raw {
				writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
				return
			}
			pageSize = value
		}
		campaignID := r.PathValue("campaign_id")
		if _, err := uuid.Parse(campaignID); err != nil {
			writeControlProblem(w, r, http.StatusNotFound, "not_found")
			return
		}
		afterRevision, afterSequence := int64(0), int64(-1)
		if cursor := query.Get("cursor"); cursor != "" {
			var err error
			afterRevision, afterSequence, err = decodeCampaignAuditCursor(cursorKey, cursor, campaignID, pageSize, now())
			if err != nil {
				writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
				return
			}
		}
		principal, ok := principalFromContext(r.Context())
		if !ok {
			writeUnauthorized(w, correlationID(r.Context()))
			return
		}
		page, err := lister.ListCampaignAudit(r.Context(), principal, campaignID, afterRevision, afterSequence, pageSize)
		if errors.Is(err, control.ErrCampaignNotFound) {
			writeControlProblem(w, r, http.StatusNotFound, "not_found")
			return
		}
		if err != nil {
			writeControlProblem(w, r, http.StatusInternalServerError, "internal_error")
			return
		}
		items := make([]AuditRecord, 0, len(page.Items))
		for _, item := range page.Items {
			items = append(items, AuditRecord{Action: item.Action, Outcome: item.Outcome, OccurredAt: item.OccurredAt})
		}
		response := AuditPage{Items: items}
		if page.HasNext {
			next, err := encodeCampaignAuditCursor(cursorKey, campaignID, page.NextAggregateRevision, page.NextEventSequence, pageSize, now())
			if err != nil {
				writeControlProblem(w, r, http.StatusInternalServerError, "internal_error")
				return
			}
			response.NextCursor = &next
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
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
			switch {
			case errors.Is(err, control.ErrInvalidCampaignCreation):
				writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
			case errors.Is(err, control.ErrCampaignRevisionConflict):
				writeControlProblem(w, r, http.StatusPreconditionFailed, "revision_conflict")
			default:
				writeControlProblem(w, r, http.StatusConflict, "command_conflict")
			}
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

func startCampaignHandler(starter campaignStarter) http.Handler {
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
		result, err := starter.StartCampaign(r.Context(), principal, control.CommandMeta{ClaimID: uuid.NewString(), CommandID: uuid.NewString(), CorrelationID: correlationID(r.Context()), OwnerToken: uuid.NewString(), OutboxEventID: uuid.NewString(), DecisionInputID: uuid.NewString(), Route: "/v1/campaigns/{campaign_id}:start", IdempotencyKey: key, CanonicalRequest: canonical}, campaignID, revision)
		if err != nil {
			if errors.Is(err, control.ErrCampaignRevisionConflict) {
				writeControlProblem(w, r, http.StatusPreconditionFailed, "revision_conflict")
			} else if errors.Is(err, control.ErrInvalidCampaignCreation) {
				writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
			} else {
				writeControlProblem(w, r, http.StatusConflict, "command_conflict")
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(CommandEnvelope{CommandId: uuid.MustParse(result.CommandID), Resource: ResourceReference{Kind: "campaign", Id: uuid.MustParse(result.CampaignID), Href: "/v1/campaigns/" + result.CampaignID}, Revision: ptr(int(result.Revision)), AcceptedAt: result.AcceptedAt})
	})
}

func pauseCampaignHandler(pauser campaignPauser) http.Handler {
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
		result, err := pauser.PauseCampaign(r.Context(), principal, control.CommandMeta{ClaimID: uuid.NewString(), CommandID: uuid.NewString(), CorrelationID: correlationID(r.Context()), OwnerToken: uuid.NewString(), OutboxEventID: uuid.NewString(), DecisionInputID: uuid.NewString(), Route: "/v1/campaigns/{campaign_id}:pause", IdempotencyKey: key, CanonicalRequest: canonical}, campaignID, revision)
		if err != nil {
			if errors.Is(err, control.ErrCampaignRevisionConflict) {
				writeControlProblem(w, r, http.StatusPreconditionFailed, "revision_conflict")
			} else if errors.Is(err, control.ErrInvalidCampaignCreation) {
				writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
			} else {
				writeControlProblem(w, r, http.StatusConflict, "command_conflict")
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(CommandEnvelope{CommandId: uuid.MustParse(result.CommandID), Resource: ResourceReference{Kind: "campaign", Id: uuid.MustParse(result.CampaignID), Href: "/v1/campaigns/" + result.CampaignID}, Revision: ptr(int(result.Revision)), AcceptedAt: result.AcceptedAt})
	})
}
