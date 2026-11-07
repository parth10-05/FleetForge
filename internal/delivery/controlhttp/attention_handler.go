package controlhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
)

type attentionResolver interface {
	ResolveAttention(context.Context, access.Principal, control.CommandMeta, string, int64, control.AttentionResolutionCommand) (control.AttentionResolutionResult, error)
}

type attentionResolverFunc func(context.Context, access.Principal, control.CommandMeta, string, int64, control.AttentionResolutionCommand) (control.AttentionResolutionResult, error)

func (fn attentionResolverFunc) ResolveAttention(ctx context.Context, principal access.Principal, meta control.CommandMeta, targetUpdateID string, expectedRevision int64, command control.AttentionResolutionCommand) (control.AttentionResolutionResult, error) {
	return fn(ctx, principal, meta, targetUpdateID, expectedRevision, command)
}

func resolveAttentionRoute(resolver attentionResolver) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/v1/target-updates/"), ":resolve-attention")
		if !ok || id == "" || strings.Contains(id, "/") {
			writeControlProblem(w, r, http.StatusNotFound, "not_found")
			return
		}
		if _, err := uuid.Parse(id); err != nil {
			writeControlProblem(w, r, http.StatusNotFound, "not_found")
			return
		}
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
		var input AttentionResolution
		if err := decodeJSON(r, &input); err != nil {
			writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
			return
		}
		principal, ok := principalFromContext(r.Context())
		if !ok {
			writeUnauthorized(w, correlationID(r.Context()))
			return
		}
		canonical, _ := json.Marshal(map[string]any{"target_update_id": id, "if_match": r.Header.Get("If-Match"), "body": input})
		command := control.AttentionResolutionCommand{Action: string(input.Action), ReasonCode: input.ReasonCode, Reason: input.Reason}
		if input.Reference != nil {
			command.Reference = *input.Reference
		}
		if input.ReservationDisposition != nil {
			command.ReservationDisposition = string(*input.ReservationDisposition)
		}
		if input.JournalConfirmedAbsent != nil {
			command.JournalConfirmedAbsent = *input.JournalConfirmedAbsent
		}
		if input.LateCallerExcluded != nil {
			command.LateCallerExcluded = *input.LateCallerExcluded
		}
		if input.LateCallerExclusionMethod != nil {
			command.LateCallerExclusionMethod = *input.LateCallerExclusionMethod
		}
		result, err := resolver.ResolveAttention(r.Context(), principal, control.CommandMeta{ClaimID: uuid.NewString(), CommandID: uuid.NewString(), CorrelationID: correlationID(r.Context()), OwnerToken: uuid.NewString(), Route: "/v1/target-updates/{target_update_id}:resolve-attention", IdempotencyKey: key, CanonicalRequest: canonical}, id, revision, command)
		if err != nil {
			if errors.Is(err, control.ErrInvalidAttentionResolution) {
				writeControlProblem(w, r, http.StatusBadRequest, "invalid_request")
			} else if errors.Is(err, control.ErrAttentionRevisionConflict) {
				writeControlProblem(w, r, http.StatusPreconditionFailed, "revision_conflict")
			} else {
				writeControlProblem(w, r, http.StatusConflict, "command_conflict")
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(CommandEnvelope{CommandId: uuid.MustParse(result.CommandID), Resource: ResourceReference{Kind: "target_update", Id: uuid.MustParse(result.TargetUpdateID), Href: "/v1/target-updates/" + result.TargetUpdateID}, Revision: ptr(int(result.Revision)), AcceptedAt: result.AcceptedAt})
	})
}
