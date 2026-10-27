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

type artifactReader interface {
	GetArtifact(context.Context, access.Principal, string) (control.ArtifactProjection, error)
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
