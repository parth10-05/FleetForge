package controlhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/parth10-05/fleetforge/internal/application/access"
	artifactapp "github.com/parth10-05/fleetforge/internal/application/artifacts"
	"github.com/parth10-05/fleetforge/internal/application/control"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

type artifactReader interface {
	GetArtifact(context.Context, access.Principal, string) (control.ArtifactProjection, error)
}

type artifactInitiator interface {
	InitiateArtifact(context.Context, access.Principal, control.CommandMeta, control.InitiateArtifactCommand) (control.ArtifactInitiationResult, artifactapp.UploadReceipt, error)
}

func initiateArtifactHandler(initiator artifactInitiator, uploadURL string, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key, err := idempotencyKey(request.Header.Get(idempotencyHeader))
		if err != nil {
			writeControlProblem(writer, request, http.StatusBadRequest, "invalid_idempotency_key")
			return
		}
		var input ArtifactInitiation
		if err := decodeJSON(request, &input); err != nil {
			writeControlProblem(writer, request, http.StatusBadRequest, "invalid_request")
			return
		}
		digest, err := value.ParseSHA256Digest("sha256:" + input.Sha256)
		if err != nil {
			writeControlProblem(writer, request, http.StatusBadRequest, "invalid_request")
			return
		}
		label := "firmware.bin"
		if input.DisplayLabel != nil {
			label = *input.DisplayLabel
		}
		metadata, err := artifact.NewUploadMetadata(label, input.MediaType, int64(input.SizeBytes), digest)
		if err != nil || strings.TrimSpace(input.FirmwareVersion) == "" {
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
		issuedAt := now().UTC()
		meta := control.CommandMeta{ClaimID: uuid.NewString(), CommandID: uuid.NewString(), CorrelationID: correlationID(request.Context()), OwnerToken: uuid.NewString(), OutboxEventID: uuid.NewString(), DecisionInputID: uuid.NewString(), Route: "/v1/artifacts:initiate", IdempotencyKey: key, CanonicalRequest: canonical}
		result, receipt, err := initiator.InitiateArtifact(request.Context(), principal, meta, control.InitiateArtifactCommand{ArtifactID: uuid.NewString(), GrantID: uuid.NewString(), FirmwareVersion: input.FirmwareVersion, Metadata: metadata, ExpiresAt: issuedAt.Add(5 * time.Minute)})
		if err != nil {
			writeControlProblem(writer, request, http.StatusConflict, "command_conflict")
			return
		}
		artifactID, err := uuid.Parse(result.ArtifactID)
		if err != nil {
			writeControlProblem(writer, request, http.StatusInternalServerError, "internal_error")
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "no-store")
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(ArtifactInitiationEnvelope{CommandId: uuid.MustParse(result.CommandID), Resource: ResourceReference{Kind: "artifact", Id: artifactID, Href: "/v1/artifacts/" + result.ArtifactID}, AcceptedAt: result.AcceptedAt, Upload: ArtifactUpload{Url: uploadURL, Grant: receipt.Authorization, ExpiresAt: receipt.ExpiresAt}})
	})
}

func getArtifactHandler(reader artifactReader) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, ok := principalFromContext(request.Context())
		if !ok {
			writeUnauthorized(writer, correlationID(request.Context()))
			return
		}
		artifactID := request.PathValue("artifact_id")
		if _, err := uuid.Parse(artifactID); err != nil {
			writeControlProblem(writer, request, http.StatusNotFound, "not_found")
			return
		}
		artifact, err := reader.GetArtifact(request.Context(), principal, artifactID)
		if errors.Is(err, control.ErrArtifactNotFound) {
			writeControlProblem(writer, request, http.StatusNotFound, "not_found")
			return
		}
		if err != nil {
			writeControlProblem(writer, request, http.StatusInternalServerError, "internal_error")
			return
		}
		id, err := uuid.Parse(artifact.ArtifactID)
		if err != nil {
			writeControlProblem(writer, request, http.StatusInternalServerError, "internal_error")
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(Artifact{Id: id, State: artifact.State, AsOf: artifact.AsOf})
	})
}
