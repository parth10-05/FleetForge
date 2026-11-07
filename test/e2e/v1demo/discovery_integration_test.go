//go:build postgresintegration

package v1demo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parth10-05/fleetforge/internal/application/access"
	appinventory "github.com/parth10-05/fleetforge/internal/application/inventory"
	"github.com/parth10-05/fleetforge/internal/delivery/controlhttp"
	"github.com/parth10-05/fleetforge/internal/delivery/redfishsim"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticdiscovery"
	"github.com/parth10-05/fleetforge/internal/redfish"
	"github.com/parth10-05/fleetforge/internal/simulator/core"
	"github.com/parth10-05/fleetforge/internal/simulator/journal"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

func TestPublicBootstrapDiscoversTarget(t *testing.T) {
	_, pool, ctx, root := acceptanceHarness(t)
	fixturePath := filepath.Join(root, "test", "fixtures", "simulator", "v1", "healthy.json")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := scenario.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := core.Start(fixture, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	writer, err := journal.OpenWriter(filepath.Join(t.TempDir(), "journal.jsonl"), "p06-discovery")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	simulator := httptest.NewServer(redfishsim.NewWithJournal(engine, []byte("user"), []byte("pass"), time.Second, 1024, writer).Routes())
	defer simulator.Close()

	key, err := scenario.ID(fixture, "target", fixture.Targets[0].Index)
	if err != nil {
		t.Fatal(err)
	}
	api := newAcceptanceAPI(t, pool, "http://127.0.0.1:1/v1/objects/upload")
	token := api.token(t, access.RoleAdmin)
	body := `{"display_name":"acceptance-discovery","endpoint_reference":"` + simulator.URL + `","target_key":"` + key + `","credential_reference":"synthetic-test:` + key + `","labels":{"site":"` + fixture.Targets[0].Site + `","rack":"` + fixture.Targets[0].Rack + `","family":"synthetic"}}`
	register := authenticatedRequest(t, http.MethodPost, api.server.URL+"/v1/targets", body, token)
	register.Header.Set("Idempotency-Key", "p06-discovery-register-0001")
	registered, err := api.server.Client().Do(register)
	if err != nil {
		t.Fatal(err)
	}
	defer registered.Body.Close()
	if registered.StatusCode != http.StatusCreated {
		t.Fatalf("registration status=%d", registered.StatusCode)
	}
	var target controlhttp.CommandEnvelope
	if err := json.NewDecoder(registered.Body).Decode(&target); err != nil {
		t.Fatal(err)
	}

	discover := authenticatedRequest(t, http.MethodPost, api.server.URL+"/v1/targets/"+target.Resource.Id.String()+":discover", "", token)
	discover.Header.Set("Idempotency-Key", "p06-discovery-request-0001")
	discover.Header.Set("If-Match", "\"rev-0\"")
	accepted, err := api.server.Client().Do(discover)
	if err != nil {
		t.Fatal(err)
	}
	defer accepted.Body.Close()
	if accepted.StatusCode != http.StatusAccepted {
		t.Fatalf("discovery status=%d", accepted.StatusCode)
	}

	credential, err := syntheticcredentials.NewCredential("user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := syntheticcredentials.New(map[string]syntheticcredentials.Credential{key: credential})
	if err != nil {
		t.Fatal(err)
	}
	adapter := persistence.DiscoveryWorkAdapter{Pool: pool}
	executor := syntheticdiscovery.Executor{Loader: adapter, Client: redfish.NewClient(redfish.ClientConfig{HTTPClient: simulator.Client(), AllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}), Resolver: resolver}
	now := time.Now().UTC()
	processed, err := appinventory.RunDiscoveryOnce(context.Background(), adapter, executor, adapter, appinventory.DiscoveryClaimRequest{Owner: "p06-acceptance", LeaseToken: uuid.NewString(), Now: now, LeaseExpiresAt: now.Add(time.Minute)}, func(_ appinventory.DiscoveryWork, value time.Time) time.Time { return value.Add(time.Second) })
	if err != nil || !processed {
		t.Fatalf("run discovery processed=%t err=%v", processed, err)
	}
	var heads, completed int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM target_discovery_heads WHERE target_id=$1::uuid", target.Resource.Id.String()).Scan(&heads); err != nil || heads != 1 {
		t.Fatalf("discovery heads=%d err=%v", heads, err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM due_work WHERE subject_id=$1::uuid AND work_type='DISCOVER' AND delivery_state='COMPLETED'", target.Resource.Id.String()).Scan(&completed); err != nil || completed != 1 {
		t.Fatalf("completed work=%d err=%v", completed, err)
	}
}
