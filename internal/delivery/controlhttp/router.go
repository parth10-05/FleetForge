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
	return NewRouterWithTargets(verifier, now, nil)
}

// NewRouterWithTargets registers only command routes whose application
// dependencies are available. Internal service routes remain outside it.
func NewRouterWithTargets(verifier access.TokenVerifier, now func() time.Time, targets targetRegistrar) http.Handler {
	if now == nil {
		now = time.Now
	}
	mux := http.NewServeMux()
	if targets != nil {
		mux.Handle("POST /v1/targets", withAuthentication(verifier)(requirePermission(access.PermissionRegisterTargets, now)(registerTargetHandler(targets))))
		if reader, ok := targets.(targetReader); ok {
			mux.Handle("GET /v1/targets/{target_id}", withAuthentication(verifier)(requirePermission(access.PermissionReadTargets, now)(getTargetHandler(reader))))
		}
		if discoverer, ok := targets.(targetDiscoverer); ok {
			mux.Handle("POST /v1/targets/", withAuthentication(verifier)(requirePermission(access.PermissionDiscoverTargets, now)(discoverTargetHandler(discoverer))))
		}
	}
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
