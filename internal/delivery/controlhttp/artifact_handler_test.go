package controlhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
)

func TestGetArtifactRouteAllowsViewerWithoutExistenceLeak(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	artifactID := "10000000-0000-0000-0000-000000000001"
	reader := &recordingArtifactReader{artifact: control.ArtifactProjection{ArtifactID: artifactID, State: "UPLOADING", AsOf: now}}
	verifier := tokenVerifierFunc(func(_ context.Context, token string) (access.Principal, error) {
		role := access.RoleViewer
		if token == "none" {
			role = "unknown"
		}
		return access.Principal{Subject: "user", Roles: []access.Role{role}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}, nil
	})
	router := NewRouterWithResources(verifier, func() time.Time { return now }, nil, reader, nil)
	request := httptest.NewRequest(http.MethodGet, "/v1/artifacts/"+artifactID, nil)
	request.Header.Set("Authorization", "Bearer viewer")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || reader.calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", recorder.Code, reader.calls, recorder.Body.String())
	}
	denied := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/artifacts/"+artifactID, nil)
	request.Header.Set("Authorization", "Bearer none")
	router.ServeHTTP(denied, request)
	if denied.Code != http.StatusForbidden || reader.calls != 1 {
		t.Fatalf("unknown-role status=%d calls=%d", denied.Code, reader.calls)
	}
	notFound := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/artifacts/10000000-0000-0000-0000-000000000002", nil)
	request.Header.Set("Authorization", "Bearer viewer")
	router.ServeHTTP(notFound, request)
	if notFound.Code != http.StatusNotFound || reader.calls != 2 {
		t.Fatalf("not-found status=%d calls=%d", notFound.Code, reader.calls)
	}
}

type recordingArtifactReader struct {
	artifact control.ArtifactProjection
	calls    int
}

func (reader *recordingArtifactReader) GetArtifact(_ context.Context, _ access.Principal, artifactID string) (control.ArtifactProjection, error) {
	reader.calls++
	if artifactID != reader.artifact.ArtifactID {
		return control.ArtifactProjection{}, control.ErrArtifactNotFound
	}
	return reader.artifact, nil
}
