package localobjecthttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
)

func TestUploadAcceptsOnlyBoundSignedRequest(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	claims := localartifactstore.UploadGrantClaims{Version: "fleetforge.upload-grant/v1", GrantID: "grant-http", Method: "PUT", Provider: "local-versioned-v1", Container: "artifacts", Key: "objects/http.bin", VersionID: "version-1", Size: 11, Digest: "sha256:b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9", MediaType: "application/octet-stream", ExpiresAt: now.Add(time.Minute)}
	grant, err := localartifactstore.IssueSignedGrant(secret, claims)
	if err != nil {
		t.Fatal(err)
	}
	store, err := localartifactstore.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := New(store, secret, func() time.Time { return now }).Routes()
	req := httptest.NewRequest(http.MethodPut, "/v1/objects/upload", bytes.NewBufferString("hello world"))
	req.Header.Set(GrantHeader, grant.Payload+"."+grant.Signature)
	req.Header.Set("Content-Type", "application/octet-stream")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	if response.Code != http.StatusCreated || response.Body.String() != `{"version_id":"version-1"}` {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	wrongType := httptest.NewRequest(http.MethodPut, "/v1/objects/upload", bytes.NewBufferString("hello world"))
	wrongType.Header.Set(GrantHeader, grant.Payload+"."+grant.Signature)
	wrongType.Header.Set("Content-Type", "text/plain")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, wrongType)
	if response.Code != http.StatusForbidden {
		t.Fatalf("wrong type status=%d", response.Code)
	}
}
