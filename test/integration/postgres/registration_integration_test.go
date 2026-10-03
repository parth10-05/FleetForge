//go:build postgresintegration

package postgres

import (
	"errors"
	"testing"

	app "github.com/parth10-05/fleetforge/internal/application/inventory"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestRegistrationIsIdempotentAndRejectsEndpointMutation(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	service := app.New(persistence.RegistrationAdapter{Pool: pool})
	request := app.RegisterTargetRequest{TargetID: "00000000-0000-0000-2101-000000000001", ExternalID: "rack-a-bmc-01", Origin: "https://simulator.example", TargetKey: "rack-a-bmc-01", CredentialReference: "synthetic-test:rack-a", TransportPolicy: inventory.SyntheticTransportPolicyV1, Labels: map[string]string{"site": "iad", "rack": "a-01", "family": "synthetic"}}
	first, err := service.RegisterTarget(ctx, request)
	if err != nil || !first.Created {
		t.Fatalf("first registration=%+v err=%v", first, err)
	}
	request.TargetID = "00000000-0000-0000-2101-000000000002"
	replay, err := service.RegisterTarget(ctx, request)
	if err != nil || replay.Created || replay.TargetID != first.TargetID {
		t.Fatalf("replay=%+v first=%+v err=%v", replay, first, err)
	}
	changed := mustRegistration(t, "https://other.example")
	_, err = persistence.RegisterTarget(ctx, pool, "00000000-0000-0000-2101-000000000003", changed)
	if !errors.Is(err, persistence.ErrRegistrationConflict) {
		t.Fatalf("changed endpoint error=%v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM targets WHERE external_identity='rack-a-bmc-01'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("target count=%d err=%v", count, err)
	}
	var labels map[string]string
	rows, err := pool.Query(ctx, "SELECT key,value FROM target_labels WHERE target_id=$1 ORDER BY key", first.TargetID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	labels = map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatal(err)
		}
		labels[key] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(labels) != 3 || labels["site"] != "iad" || labels["rack"] != "a-01" || labels["family"] != "synthetic" {
		t.Fatalf("stored labels=%v", labels)
	}
}

func mustRegistration(t *testing.T, endpoint string) inventory.Registration {
	t.Helper()
	registration, err := inventory.NewRegistration("rack-a-bmc-01", endpoint, "rack-a-bmc-01", inventory.SyntheticContractV1, "synthetic-test:rack-a", inventory.SyntheticTransportPolicyV1, map[string]string{"site": "iad", "rack": "a-01", "family": "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	return registration
}
