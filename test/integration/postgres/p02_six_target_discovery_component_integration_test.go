//go:build postgresintegration

package postgres

import (
	"fmt"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	appinventory "github.com/parth10-05/fleetforge/internal/application/inventory"
	"github.com/parth10-05/fleetforge/internal/delivery/inventoryadapter"
	"github.com/parth10-05/fleetforge/internal/delivery/redfishsim"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticdiscovery"
	"github.com/parth10-05/fleetforge/internal/redfish"
	"github.com/parth10-05/fleetforge/internal/simulator/core"
	"github.com/parth10-05/fleetforge/internal/simulator/journal"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

// TestP02SixTargetDiscoveryComponentIsReadOnly composes the P02 registration
// and discovery adapters with P01's authenticated six-target simulator. It
// deliberately never reaches UpdateService: the retained independent journal
// must stay empty of firmware requests and accepted operations.
func TestP02SixTargetDiscoveryComponentIsReadOnly(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	raw, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "simulator", "v1", "healthy.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := scenario.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := core.Start(s, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	journalPath := filepath.Join(t.TempDir(), "journal.jsonl")
	writer, err := journal.OpenWriter(journalPath, "p02-six-target-discovery")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	server := httptest.NewServer(redfishsim.NewWithJournal(engine, []byte("user"), []byte("pass"), time.Second, 1024, writer).Routes())
	defer server.Close()

	credential, err := syntheticcredentials.NewCredential("user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	credentials := make(map[string]syntheticcredentials.Credential, len(s.Targets))
	keys := make([]string, 0, len(s.Targets))
	for _, target := range s.Targets {
		key, idErr := scenario.ID(s, "target", target.Index)
		if idErr != nil {
			t.Fatal(idErr)
		}
		keys = append(keys, key)
		credentials[key] = credential
	}
	resolver, err := syntheticcredentials.New(credentials)
	if err != nil {
		t.Fatal(err)
	}
	registration := appinventory.New(inventoryadapter.PostgresRegistrationWriter{Pool: pool})
	now := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	for index, target := range s.Targets {
		targetID := fmt.Sprintf("00000000-0000-0000-3001-%012d", index+1)
		if _, err := registration.RegisterTarget(ctx, appinventory.RegisterTargetRequest{TargetID: targetID, ExternalID: "component-" + keys[index], Origin: server.URL, TargetKey: keys[index], CredentialReference: "synthetic-test:" + keys[index], TransportPolicy: "synthetic-allowlisted-v1", Labels: map[string]string{"site": target.Site, "rack": target.Rack, "family": "synthetic"}}); err != nil {
			t.Fatalf("register target %d: %v", index+1, err)
		}
		// The discovery worker's persisted contract is versioned separately from
		// registration's allow-list policy. Its current reader requires the
		// discovery contract literal, so retain that explicit configuration
		// boundary in the component fixture.
		if _, err := pool.Exec(ctx, "UPDATE targets SET transport_policy_version='synthetic-redfish-v1' WHERE id=$1::uuid", targetID); err != nil {
			t.Fatalf("configure discovery contract for target %d: %v", index+1, err)
		}
		workID := fmt.Sprintf("00000000-0000-0000-3002-%012d", index+1)
		commandID := fmt.Sprintf("00000000-0000-0000-3003-%012d", index+1)
		if ok, err := postgres.ScheduleDiscovery(ctx, pool, postgres.DiscoveryScheduleRequest{WorkID: workID, CommandID: commandID, TargetID: targetID, TargetRevision: 0, NotBefore: now}); err != nil || !ok {
			t.Fatalf("schedule target %d: ok=%t err=%v", index+1, ok, err)
		}
	}

	adapter := postgres.DiscoveryWorkAdapter{Pool: pool}
	observation := 0
	executor := syntheticdiscovery.Executor{Loader: adapter, Client: redfish.NewClient(redfish.ClientConfig{HTTPClient: server.Client(), AllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}), Resolver: resolver, Now: func() time.Time { return now }, NewObservationID: func() (string, error) {
		observation++
		return fmt.Sprintf("00000000-0000-0000-3004-%012d", observation), nil
	}}
	for i := 0; i < len(s.Targets); i++ {
		processed, err := appinventory.RunDiscoveryOnce(ctx, adapter, executor, adapter, appinventory.DiscoveryClaimRequest{Owner: "component-worker", LeaseToken: fmt.Sprintf("00000000-0000-0000-3005-%012d", i+1), Now: now, LeaseExpiresAt: now.Add(time.Minute)}, retryOneSecond)
		if err != nil || !processed {
			t.Fatalf("discover target %d: processed=%t err=%v", i+1, processed, err)
		}
	}
	var heads, completed int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM target_discovery_heads").Scan(&heads); err != nil || heads != 6 {
		t.Fatalf("discovery heads=%d err=%v", heads, err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM due_work WHERE work_type='DISCOVER' AND delivery_state='COMPLETED'").Scan(&completed); err != nil || completed != 6 {
		t.Fatalf("completed discovery work=%d err=%v", completed, err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := journal.Load(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Kind == journal.RequestReceived || entry.Kind == journal.UpdateAccepted {
			t.Fatalf("P02 read-only component wrote firmware journal entry: %#v", entry)
		}
	}
}
