//go:build postgresintegration

package v1demo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parth10-05/fleetforge/internal/application/access"
	artifactapp "github.com/parth10-05/fleetforge/internal/application/artifacts"
	"github.com/parth10-05/fleetforge/internal/delivery/controlhttp"
	"github.com/parth10-05/fleetforge/internal/delivery/localobjecthttp"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
)

func TestPublicArtifactUploadSchedulesAndCompletesVerification(t *testing.T) {
	_, pool, ctx, _ := acceptanceHarness(t)
	key := []byte("01234567890123456789012345678901")
	store, err := localartifactstore.OpenDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	completion := artifactapp.NewWithCompletion(nil, nil, store, persistence.ArtifactBindingAdapter{Pool: pool}, nil)
	upload := httptest.NewServer(localobjecthttp.New(store, key, time.Now).WithCompletion(completion).Routes())
	defer upload.Close()
	api := newAcceptanceAPI(t, pool, upload.URL+"/v1/objects/upload")
	data := []byte("abc")
	digest := sha256.Sum256(data)
	request := authenticatedRequest(t, http.MethodPost, api.server.URL+"/v1/artifacts:initiate", `{"sha256":"`+hex.EncodeToString(digest[:])+`","size_bytes":3,"media_type":"application/octet-stream","firmware_version":"v1"}`, api.token(t, access.RoleAdmin))
	request.Header.Set("Idempotency-Key", "p06-artifact-initiate-0001")
	response, err := api.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("initiate status=%d", response.StatusCode)
	}
	var initiated controlhttp.ArtifactInitiationEnvelope
	if err := json.NewDecoder(response.Body).Decode(&initiated); err != nil {
		t.Fatal(err)
	}
	uploadRequest, err := http.NewRequestWithContext(ctx, http.MethodPut, initiated.Upload.Url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	uploadRequest.Header.Set(localobjecthttp.GrantHeader, initiated.Upload.Grant)
	uploadRequest.Header.Set("Content-Type", "application/octet-stream")
	uploadResponse, err := upload.Client().Do(uploadRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer uploadResponse.Body.Close()
	if uploadResponse.StatusCode != http.StatusCreated {
		t.Fatalf("upload status=%d", uploadResponse.StatusCode)
	}

	verification := persistence.ArtifactVerificationAdapter{Pool: pool}
	worker := artifactapp.NewWorker(artifactapp.NewWithCompletion(nil, nil, store, nil, nil).WithVerification(verification, verification), persistence.ArtifactVerificationWorkAdapter{Pool: pool}, persistence.ArtifactVerificationWorkAdapter{Pool: pool}, time.Now)
	now := time.Now().UTC()
	if err := worker.RunOnce(ctx, artifactapp.VerificationWorkClaimRequest{Owner: "p06-acceptance", LeaseToken: uuid.NewString(), Now: now, LeaseExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	read := authenticatedRequest(t, http.MethodGet, api.server.URL+"/v1/artifacts/"+initiated.Resource.Id.String(), "", api.token(t, access.RoleAdmin))
	readResponse, err := api.server.Client().Do(read)
	if err != nil {
		t.Fatal(err)
	}
	defer readResponse.Body.Close()
	if readResponse.StatusCode != http.StatusOK {
		t.Fatalf("read status=%d", readResponse.StatusCode)
	}
	var projection controlhttp.Artifact
	if err := json.NewDecoder(readResponse.Body).Decode(&projection); err != nil {
		t.Fatal(err)
	}
	if projection.State != "VERIFIED" {
		t.Fatalf("artifact state=%q", projection.State)
	}
	var pending, completed int
	if err := pool.QueryRow(ctx, "SELECT count(*) FILTER (WHERE delivery_state='PENDING'),count(*) FILTER (WHERE delivery_state='COMPLETED') FROM due_work WHERE subject_id=$1::uuid AND subject_type='ARTIFACT' AND work_type='VERIFY_ARTIFACT'", initiated.Resource.Id.String()).Scan(&pending, &completed); err != nil || pending != 0 || completed != 1 {
		t.Fatalf("verification work pending=%d completed=%d err=%v", pending, completed, err)
	}
}
