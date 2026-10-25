package controlhttp

import (
	"net/http"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
)

// NewRouter owns only the public human control API namespace. The caller must
// mount internal service routes separately so bearer and HMAC credentials can
// never become interchangeable.
func NewRouter(verifier access.TokenVerifier, now func() time.Time) http.Handler {
	if now == nil {
		now = time.Now
	}
	mux := http.NewServeMux()
	mux.Handle("/v1/", withAuthentication(verifier)(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeProblem(writer, Problem{
			Type:          "https://fleetforge.local/problems/not-found",
			Title:         "Not found",
			Status:        http.StatusNotFound,
			Code:          "not_found",
			CorrelationId: responseCorrelationID(correlationID(request.Context())),
		})
	})))
	return withCorrelation(withJSONBodyLimit(mux))
}
