package controlhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

const (
	correlationHeader = "X-Correlation-ID"
	maxJSONBodyBytes  = 64 << 10
)

type correlationKey struct{}

func correlationID(ctx context.Context) string {
	value, _ := ctx.Value(correlationKey{}).(string)
	return value
}

func withCorrelation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		id := request.Header.Get(correlationHeader)
		if id == "" {
			id = uuid.NewString()
		} else if parsed, err := uuid.Parse(id); err != nil || parsed.String() != id {
			problemID := uuid.New()
			writeProblem(writer, Problem{Type: "https://fleetforge.local/problems/invalid-correlation-id", Title: "Invalid request", Status: http.StatusBadRequest, Code: "invalid_correlation_id", CorrelationId: problemID})
			return
		}
		writer.Header().Set(correlationHeader, id)
		next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), correlationKey{}, id)))
	})
}

func withJSONBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Body != nil {
			request.Body = http.MaxBytesReader(writer, request.Body, maxJSONBodyBytes)
		}
		next.ServeHTTP(writer, request)
	})
}

func decodeJSON(request *http.Request, destination any) error {
	if !strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("content type must be application/json")
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("request must contain exactly one JSON value")
	}
	return nil
}
