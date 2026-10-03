package syntheticdiscovery

import (
	"bytes"
	"context"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	app "github.com/parth10-05/fleetforge/internal/application/inventory"
	"github.com/parth10-05/fleetforge/internal/delivery/redfishsim"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
	"github.com/parth10-05/fleetforge/internal/redfish"
	"github.com/parth10-05/fleetforge/internal/simulator/core"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

func TestExecutorProducesNormalizedReadOnlyDiscoveryEvidence(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	credential, err := syntheticcredentials.NewCredential("user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := syntheticcredentials.New(map[string]syntheticcredentials.Credential{"rack-a": credential})
	if err != nil {
		t.Fatal(err)
	}
	client := &serviceClient{capabilities: redfish.ServiceCapabilities{RedfishVersion: "1.16.0", SystemsURI: "/Systems", ManagersURI: "/Managers", UpdateServiceURI: "/UpdateService"}}
	executor := Executor{Loader: targetLoader{target: app.DiscoveryTarget{Origin: "https://simulator.example", TargetKey: "rack-a", CredentialReference: "synthetic-test:rack-a"}}, Client: client, Resolver: resolver, Now: func() time.Time { return now }, NewObservationID: func() (string, error) { return "00000000-0000-0000-2901-000000000001", nil }}
	result, err := executor.Discover(context.Background(), app.DiscoveryWork{ID: "work", TargetID: "target", ExpectedRevision: 0})
	if err != nil || !result.Success || result.ObservationID == "" || !result.ObservedAt.Equal(now) || string(result.Normalized) != `{"redfish_version":"1.16.0","systems_uri":"/Systems","managers_uri":"/Managers","update_service_uri":"/UpdateService"}` {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if client.address.ServiceRoot() != "https://simulator.example/virtual-bmcs/rack-a/redfish/v1/" {
		t.Fatalf("service root=%q", client.address.ServiceRoot())
	}
}

func TestExecutorDiscoversAgainstAuthenticatedSyntheticRuntime(t *testing.T) {
	s := scenario.Scenario{SchemaVersion: scenario.SchemaVersionV1, ScenarioID: "discovery-executor-runtime", Seed: "11", Topology: scenario.Topology{Sites: []string{"iad"}}, Targets: []scenario.Target{{Index: 1, Site: "iad", Rack: "a-01", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 2, Site: "iad", Rack: "a-02", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}}, Timing: scenario.Timing{UploadMS: 1}, Limits: scenario.Limits{CallbackWorkers: 1, Subscriptions: 1, BodyBytes: 1024, JournalEntries: 1, OpenConnections: 1, EventsPerSecond: 1}}
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
	credential, err := syntheticcredentials.NewCredential("user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := syntheticcredentials.New(map[string]syntheticcredentials.Credential{"runtime-target": credential})
	if err != nil {
		t.Fatal(err)
	}
	executor := Executor{
		Loader:           targetLoader{target: app.DiscoveryTarget{Origin: server.URL, TargetKey: targetKey, CredentialReference: "synthetic-test:runtime-target"}},
		Client:           redfish.NewClient(redfish.ClientConfig{HTTPClient: server.Client(), AllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}),
		Resolver:         resolver,
		Now:              func() time.Time { return time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC) },
		NewObservationID: func() (string, error) { return "00000000-0000-0000-2901-000000000003", nil },
	}
	result, err := executor.Discover(context.Background(), app.DiscoveryWork{ID: "work", TargetID: "target", ExpectedRevision: 0})
	if err != nil || !result.Success || result.ObservationID != "00000000-0000-0000-2901-000000000003" || string(result.Normalized) == "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if want := []byte(`"redfish_version":"1.16.0"`); !bytes.Contains(result.Normalized, want) {
		t.Fatalf("normalized=%s missing %s", result.Normalized, want)
	}
}

func TestExecutorRetriesOnlyRetryableRedfishFailures(t *testing.T) {
	credential, _ := syntheticcredentials.NewCredential("user", "pass")
	resolver, _ := syntheticcredentials.New(map[string]syntheticcredentials.Credential{"rack-a": credential})
	executor := Executor{Loader: targetLoader{target: app.DiscoveryTarget{Origin: "https://simulator.example", TargetKey: "rack-a", CredentialReference: "synthetic-test:rack-a"}}, Client: &serviceClient{err: redfish.DiscoveryError{Kind: redfish.ErrorTimeout}}, Resolver: resolver}
	if _, err := executor.Discover(context.Background(), app.DiscoveryWork{ID: "work", TargetID: "target", ExpectedRevision: 0}); err == nil {
		t.Fatal("retryable timeout was turned into a durable failure")
	}
}

func TestExecutorRecordsTerminalRedfishFailureWithoutLeakingTransportError(t *testing.T) {
	credential, _ := syntheticcredentials.NewCredential("user", "pass")
	resolver, _ := syntheticcredentials.New(map[string]syntheticcredentials.Credential{"rack-a": credential})
	executor := Executor{Loader: targetLoader{target: app.DiscoveryTarget{Origin: "https://simulator.example", TargetKey: "rack-a", CredentialReference: "synthetic-test:rack-a"}}, Client: &serviceClient{err: redfish.DiscoveryError{Kind: redfish.ErrorUnauthorized}}, Resolver: resolver, NewObservationID: func() (string, error) { return "00000000-0000-0000-2901-000000000002", nil }}
	result, err := executor.Discover(context.Background(), app.DiscoveryWork{ID: "work", TargetID: "target", ExpectedRevision: 0})
	if err != nil || result.Success || string(result.Normalized) != `{"error_kind":"unauthorized"}` {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

type targetLoader struct {
	target app.DiscoveryTarget
	err    error
}

func (loader targetLoader) LoadDiscoveryTarget(context.Context, app.DiscoveryWork) (app.DiscoveryTarget, error) {
	return loader.target, loader.err
}

type serviceClient struct {
	address      redfish.TargetAddress
	capabilities redfish.ServiceCapabilities
	err          error
}

func (client *serviceClient) DiscoverServiceAuthenticated(_ context.Context, address redfish.TargetAddress, _ redfish.RequestAuthenticator) (redfish.ServiceCapabilities, error) {
	client.address = address
	if client.err != nil {
		return redfish.ServiceCapabilities{}, client.err
	}
	return client.capabilities, nil
}
