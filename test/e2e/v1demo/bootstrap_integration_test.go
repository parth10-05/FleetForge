//go:build postgresintegration

package v1demo

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/delivery/controlhttp"
)

// TestPublicBootstrapRegistersSyntheticTarget proves the acceptance package
// begins with an authenticated public command, not a direct lifecycle insert.
func TestPublicBootstrapRegistersSyntheticTarget(t *testing.T) {
	_, pool, _, _ := acceptanceHarness(t)
	api := newAcceptanceAPI(t, pool, "http://127.0.0.1:1/v1/objects/upload")
	token := api.token(t, access.RoleAdmin)
	body := `{"display_name":"acceptance-target","endpoint_reference":"http://127.0.0.1:18081","target_key":"target-1","credential_reference":"synthetic-test:target-1","labels":{"site":"west","rack":"rack-a","family":"synthetic"}}`
	request := authenticatedRequest(t, http.MethodPost, api.server.URL+"/v1/targets", body, token)
	request.Header.Set("Idempotency-Key", "p06-bootstrap-target-0001")
	response, err := api.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("registration status=%d", response.StatusCode)
	}
	var created controlhttp.CommandEnvelope
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	replay := authenticatedRequest(t, http.MethodPost, api.server.URL+"/v1/targets", body, token)
	replay.Header.Set("Idempotency-Key", "p06-bootstrap-target-0001")
	replayResponse, err := api.server.Client().Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	defer replayResponse.Body.Close()
	if replayResponse.StatusCode != http.StatusCreated {
		t.Fatalf("replay status=%d", replayResponse.StatusCode)
	}
	var replayed controlhttp.CommandEnvelope
	if err := json.NewDecoder(replayResponse.Body).Decode(&replayed); err != nil {
		t.Fatal(err)
	}
	if replayed.Resource.Id != created.Resource.Id || replayed.CommandId != created.CommandId {
		t.Fatalf("replay=%+v created=%+v", replayed, created)
	}
}
