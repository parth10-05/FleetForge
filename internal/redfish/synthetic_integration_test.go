package redfish

import (
	"context"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/delivery/redfishsim"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
	"github.com/parth10-05/fleetforge/internal/simulator/core"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

func TestAuthenticatedDiscoveryAgainstSyntheticSimulator(t *testing.T) {
	s := scenario.Scenario{SchemaVersion: scenario.SchemaVersionV1, ScenarioID: "bounded-discovery", Seed: "1", Topology: scenario.Topology{Sites: []string{"iad"}}, Targets: []scenario.Target{{Index: 1, Site: "iad", Rack: "a-01", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 2, Site: "iad", Rack: "a-02", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}}, Timing: scenario.Timing{UploadMS: 1}, Limits: scenario.Limits{CallbackWorkers: 1, Subscriptions: 1, BodyBytes: 1024, JournalEntries: 1, OpenConnections: 1, EventsPerSecond: 1}}
	engine, err := core.Start(s, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	targetKey, err := scenario.ID(s, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(redfishsim.New(engine, []byte("user"), []byte("pass"), time.Second, 1024).Routes())
	defer server.Close()
	credential, _ := syntheticcredentials.NewCredential("user", "pass")
	resolver, err := syntheticcredentials.New(map[string]syntheticcredentials.Credential{"rack-a": credential})
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := resolver.Resolve("synthetic-test:rack-a")
	if err != nil {
		t.Fatal(err)
	}
	address, err := ParseTargetAddress(server.URL, targetKey)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(ClientConfig{HTTPClient: server.Client(), AllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
	capabilities, err := client.DiscoverServiceAuthenticated(context.Background(), address, authenticator)
	if err != nil {
		t.Fatal(err)
	}
	if capabilities.RedfishVersion != "1.16.0" || capabilities.SystemsURI == "" || capabilities.UpdateServiceURI == "" {
		t.Fatalf("capabilities=%+v", capabilities)
	}
}
