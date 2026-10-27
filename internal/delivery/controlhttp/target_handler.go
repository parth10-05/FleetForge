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

type targetRegistrar interface {
	RegisterTarget(context.Context, access.Principal, control.CommandMeta, control.RegisterTargetCommand) (control.TargetRegistrationResult, error)
}

type targetReader interface {
	GetTarget(context.Context, access.Principal, string) (control.TargetRegistrationResult, string, error)
}

type targetDiscoverer interface {
	DiscoverTarget(context.Context, access.Principal, control.CommandMeta, string, int64) (control.TargetRegistrationResult, error)
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

func discoverTargetHandler(discoverer targetDiscoverer) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key, err := idempotencyKey(request.Header.Get(idempotencyHeader))
		if err != nil {
			writeControlProblem(writer, request, http.StatusBadRequest, "invalid_idempotency_key")
			return
		}
		if request.Header.Get("If-Match") == "" {
			writeControlProblem(writer, request, http.StatusPreconditionRequired, "precondition_required")
			return
		}
		revision, err := parseRevisionETag(request.Header.Get("If-Match"))
		if err != nil {
			writeControlProblem(writer, request, http.StatusBadRequest, "invalid_precondition")
			return
		}
		principal, ok := principalFromContext(request.Context())
		if !ok {
			writeUnauthorized(writer, correlationID(request.Context()))
			return
		}
		targetID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/v1/targets/"), ":discover")
		if !strings.HasSuffix(request.URL.Path, ":discover") || strings.Contains(targetID, "/") {
			writeControlProblem(writer, request, http.StatusNotFound, "not_found")
			return
		}
		if _, err := uuid.Parse(targetID); err != nil {
			writeControlProblem(writer, request, http.StatusNotFound, "not_found")
			return
		}
		canonical, _ := json.Marshal(map[string]string{"if_match": request.Header.Get("If-Match"), "target_id": targetID})
		meta := control.CommandMeta{ClaimID: uuid.NewString(), CommandID: uuid.NewString(), CorrelationID: correlationID(request.Context()), OwnerToken: uuid.NewString(), OutboxEventID: uuid.NewString(), DecisionInputID: uuid.NewString(), WorkID: uuid.NewString(), Route: "/v1/targets/{target_id}:discover", IdempotencyKey: key, CanonicalRequest: canonical}
		result, err := discoverer.DiscoverTarget(request.Context(), principal, meta, targetID, revision)
		if errors.Is(err, control.ErrTargetRevisionConflict) {
			writeControlProblem(writer, request, http.StatusPreconditionFailed, "revision_conflict")
			return
		}
		if err != nil {
			writeControlProblem(writer, request, http.StatusConflict, "command_conflict")
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(writer).Encode(CommandEnvelope{CommandId: uuid.MustParse(result.CommandID), Resource: ResourceReference{Kind: "target", Id: uuid.MustParse(result.TargetID), Href: "/v1/targets/" + result.TargetID}, Revision: ptr(int(result.Revision)), AcceptedAt: result.AcceptedAt})
	})
}

func ptr[T any](value T) *T { return &value }

func writeControlProblem(writer http.ResponseWriter, request *http.Request, status int, code string) {
	writeProblem(writer, Problem{Type: "https://fleetforge.local/problems/" + code, Title: http.StatusText(status), Status: status, Code: code, CorrelationId: responseCorrelationID(correlationID(request.Context()))})
}
