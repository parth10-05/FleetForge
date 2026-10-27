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
	return NewRouterWithTargetsAndCursor(verifier, now, targets, nil)
}

// NewRouterWithTargetsAndCursor additionally enables signed target pagination.
func NewRouterWithTargetsAndCursor(verifier access.TokenVerifier, now func() time.Time, targets targetRegistrar, cursorKey []byte) http.Handler {
	return NewRouterWithResources(verifier, now, targets, nil, cursorKey)
}

// NewRouterWithResources registers the available human control API resources.
// Internal service routes remain outside it.
func NewRouterWithResources(verifier access.TokenVerifier, now func() time.Time, targets targetRegistrar, artifacts artifactReader, cursorKey []byte) http.Handler {
	return NewRouterWithResourcesAndUpload(verifier, now, targets, artifacts, cursorKey, "")
}

// NewRouterWithResourcesAndUpload additionally mounts artifact initiation when
// the independent local upload endpoint has been configured.
func NewRouterWithResourcesAndUpload(verifier access.TokenVerifier, now func() time.Time, targets targetRegistrar, artifacts artifactReader, cursorKey []byte, uploadURL string) http.Handler {
	return NewRouterWithResourcesUploadAndCampaigns(verifier, now, targets, artifacts, nil, cursorKey, uploadURL)
}

// NewRouterWithResourcesUploadAndCampaigns mounts the available public control resources.
func NewRouterWithResourcesUploadAndCampaigns(verifier access.TokenVerifier, now func() time.Time, targets targetRegistrar, artifacts artifactReader, campaigns campaignCreator, cursorKey []byte, uploadURL string) http.Handler {
	if now == nil {
		now = time.Now
	}
	mux := http.NewServeMux()
	if targets != nil {
		mux.Handle("POST /v1/targets", withAuthentication(verifier)(requirePermission(access.PermissionRegisterTargets, now)(registerTargetHandler(targets))))
		if lister, ok := targets.(targetLister); ok && len(cursorKey) >= 32 {
			mux.Handle("GET /v1/targets", withAuthentication(verifier)(requirePermission(access.PermissionReadTargets, now)(listTargetsHandler(lister, cursorKey))))
		}
		if reader, ok := targets.(targetReader); ok {
			mux.Handle("GET /v1/targets/{target_id}", withAuthentication(verifier)(requirePermission(access.PermissionReadTargets, now)(getTargetHandler(reader))))
		}
		if discoverer, ok := targets.(targetDiscoverer); ok {
			mux.Handle("POST /v1/targets/", withAuthentication(verifier)(requirePermission(access.PermissionDiscoverTargets, now)(discoverTargetHandler(discoverer))))
		}
	}
	if artifacts != nil {
		mux.Handle("GET /v1/artifacts/{artifact_id}", withAuthentication(verifier)(requirePermission(access.PermissionReadArtifacts, now)(getArtifactHandler(artifacts))))
		if initiator, ok := artifacts.(artifactInitiator); ok && uploadURL != "" {
			mux.Handle("POST /v1/artifacts:initiate", withAuthentication(verifier)(requirePermission(access.PermissionInitiateArtifacts, now)(initiateArtifactHandler(initiator, uploadURL, now))))
		}
	}
	if campaigns != nil {
		mux.Handle("POST /v1/campaigns", withAuthentication(verifier)(requirePermission(access.PermissionCreateCampaigns, now)(createCampaignHandler(campaigns))))
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
