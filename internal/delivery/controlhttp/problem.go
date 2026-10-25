package controlhttp

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

const problemContentType = "application/problem+json"

func writeProblem(writer http.ResponseWriter, problem Problem) {
	writer.Header().Set("Content-Type", problemContentType)
	writer.Header().Set("X-Correlation-ID", uuid.UUID(problem.CorrelationId).String())
	writer.WriteHeader(problem.Status)
	_ = json.NewEncoder(writer).Encode(problem)
}
