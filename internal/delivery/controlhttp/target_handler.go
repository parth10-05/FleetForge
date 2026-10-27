package controlhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
)

type targetRegistrar interface {
	RegisterTarget(context.Context, access.Principal, control.CommandMeta, control.RegisterTargetCommand) (control.TargetRegistrationResult, error)
}

type targetReader interface {
	GetTarget(context.Context, access.Principal, string) (control.TargetRegistrationResult, string, error)
}

func registerTargetHandler(registrar targetRegistrar) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key, err := idempotencyKey(request.Header.Get(idempotencyHeader))
		if err != nil {
			writeControlProblem(writer, request, http.StatusBadRequest, "invalid_idempotency_key")
			return
		}
		var input TargetRegistration
		if err := decodeJSON(request, &input); err != nil {
			writeControlProblem(writer, request, http.StatusBadRequest, "invalid_request")
			return
		}
		canonical, err := json.Marshal(input)
		if err != nil {
			writeControlProblem(writer, request, http.StatusBadRequest, "invalid_request")
			return
		}
		principal, ok := principalFromContext(request.Context())
		if !ok {
			writeUnauthorized(writer, correlationID(request.Context()))
			return
		}
		commandID, targetID := uuid.NewString(), uuid.NewString()
		result, err := registrar.RegisterTarget(request.Context(), principal, control.CommandMeta{ClaimID: uuid.NewString(), CommandID: commandID, CorrelationID: correlationID(request.Context()), OwnerToken: uuid.NewString(), OutboxEventID: uuid.NewString(), DecisionInputID: uuid.NewString(), Route: "/v1/targets", IdempotencyKey: key, CanonicalRequest: canonical}, control.RegisterTargetCommand{TargetID: targetID, ExternalID: input.DisplayName, Origin: input.EndpointReference, TargetKey: input.TargetKey, CredentialReference: input.CredentialReference, Labels: input.Labels})
		if err != nil {
			if errors.Is(err, control.ErrInvalidTargetRegistration) {
				writeControlProblem(writer, request, http.StatusBadRequest, "invalid_request")
				return
			}
			writeControlProblem(writer, request, http.StatusConflict, "command_conflict")
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(CommandEnvelope{CommandId: uuid.MustParse(result.CommandID), Resource: ResourceReference{Kind: "target", Id: uuid.MustParse(result.TargetID), Href: "/v1/targets/" + result.TargetID}, Revision: nil, AcceptedAt: result.AcceptedAt})
	})
}

func getTargetHandler(reader targetReader) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, ok := principalFromContext(request.Context())
		if !ok {
			writeUnauthorized(writer, correlationID(request.Context()))
			return
		}
		targetID := request.PathValue("target_id")
		if _, err := uuid.Parse(targetID); err != nil {
			writeControlProblem(writer, request, http.StatusNotFound, "not_found")
			return
		}
		result, state, err := reader.GetTarget(request.Context(), principal, targetID)
		if errors.Is(err, control.ErrTargetNotFound) {
			writeControlProblem(writer, request, http.StatusNotFound, "not_found")
			return
		}
		if err != nil {
			writeControlProblem(writer, request, http.StatusInternalServerError, "internal_error")
			return
		}
		etag, err := revisionETag(result.Revision)
		if err != nil {
			writeControlProblem(writer, request, http.StatusInternalServerError, "internal_error")
			return
		}
		targetUUID, err := uuid.Parse(result.TargetID)
		if err != nil {
			writeControlProblem(writer, request, http.StatusInternalServerError, "internal_error")
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("ETag", etag)
		_ = json.NewEncoder(writer).Encode(Target{Id: targetUUID, Revision: int(result.Revision), AsOf: result.AcceptedAt, State: state})
	})
}

func writeControlProblem(writer http.ResponseWriter, request *http.Request, status int, code string) {
	writeProblem(writer, Problem{Type: "https://fleetforge.local/problems/" + code, Title: http.StatusText(status), Status: status, Code: code, CorrelationId: responseCorrelationID(correlationID(request.Context()))})
}
