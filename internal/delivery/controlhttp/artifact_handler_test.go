package controlhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/access"
	artifactapp "github.com/parth10-05/fleetforge/internal/application/artifacts"
	"github.com/parth10-05/fleetforge/internal/application/control"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
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

func TestInitiateArtifactRouteRequiresAdminAndReturnsNoStoreGrant(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	service := &recordingArtifactService{reader: recordingArtifactReader{artifact: control.ArtifactProjection{ArtifactID: "10000000-0000-0000-0000-000000000001", State: "UPLOADING", AsOf: now}}}
	verifier := tokenVerifierFunc(func(_ context.Context, token string) (access.Principal, error) {
		role := access.RoleViewer
		if token == "admin" {
			role = access.RoleAdmin
		}
		return access.Principal{Subject: "user", Roles: []access.Role{role}, AuthenticatedAt: now, ExpiresAt: now.Add(time.Hour), IssuerClass: "local"}, nil
	})
	router := NewRouterWithResourcesAndUpload(verifier, func() time.Time { return now }, nil, service, nil, "http://127.0.0.1:8090/v1/objects/upload")
	body := `{"sha256":"` + strings.Repeat("a", 64) + `","size_bytes":3,"media_type":"application/octet-stream","firmware_version":"v1"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/artifacts:initiate", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer admin")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(idempotencyHeader, "artifact-initiation-request")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated || service.calls != 1 || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d calls=%d cache=%q body=%s", recorder.Code, service.calls, recorder.Header().Get("Cache-Control"), recorder.Body.String())
	}
	var response ArtifactInitiationEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Upload.Grant == "" || response.Upload.Url != "http://127.0.0.1:8090/v1/objects/upload" {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	request = httptest.NewRequest(http.MethodPost, "/v1/artifacts:initiate", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer viewer")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(idempotencyHeader, "artifact-initiation-request")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || service.calls != 1 {
		t.Fatalf("viewer status=%d calls=%d", recorder.Code, service.calls)
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

type recordingArtifactService struct {
	reader recordingArtifactReader
	calls  int
}

func (service *recordingArtifactService) GetArtifact(ctx context.Context, principal access.Principal, artifactID string) (control.ArtifactProjection, error) {
	return service.reader.GetArtifact(ctx, principal, artifactID)
}

func (service *recordingArtifactService) InitiateArtifact(_ context.Context, _ access.Principal, meta control.CommandMeta, command control.InitiateArtifactCommand) (control.ArtifactInitiationResult, artifactapp.UploadReceipt, error) {
	service.calls++
	digest, _ := value.ParseSHA256Digest("sha256:" + strings.Repeat("a", 64))
	object, _ := artifact.NewObjectIdentity(artifact.ObjectProviderLocalV1, "artifacts", "artifacts/v1/a.bin", "api-version", 3, digest)
	return control.ArtifactInitiationResult{ArtifactID: "10000000-0000-0000-0000-000000000001", CommandID: meta.CommandID, AcceptedAt: command.ExpiresAt.Add(-time.Minute)}, artifactapp.UploadReceipt{Authorization: "signed-grant", Object: object, ExpiresAt: command.ExpiresAt}, nil
}
