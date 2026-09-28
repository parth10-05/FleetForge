//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

func TestP02TargetSelectionIsParameterizedOrderedAndBounded(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	registerSelectionTarget(t, ctx, pool, "00000000-0000-0000-2601-000000000002", "selection-b", "rack-b", "west")
	registerSelectionTarget(t, ctx, pool, "00000000-0000-0000-2601-000000000001", "selection-a", "rack-a", "west")
	registerSelectionTarget(t, ctx, pool, "00000000-0000-0000-2601-000000000003", "selection-c", "rack-c", "east")
	selector, err := inventory.ParseSelector([]byte(`{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"west"},{"key":"family","value":"synthetic"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := persistence.SelectTargets(ctx, pool, selector, 2)
	if err != nil || len(selected) != 2 {
		t.Fatalf("selected=%v err=%v", selected, err)
	}
	if selected[0].ID.String() != "00000000-0000-0000-2601-000000000001" || selected[1].ID.String() != "00000000-0000-0000-2601-000000000002" {
		t.Fatalf("selected IDs=%s,%s", selected[0].ID.String(), selected[1].ID.String())
	}
	if _, err := persistence.SelectTargets(ctx, pool, selector, 1); !errors.Is(err, persistence.ErrSelectorResultLimit) {
		t.Fatalf("overflow error=%v", err)
	}
}

func registerSelectionTarget(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id, externalID, rack, site string) {
	t.Helper()
	registration, err := inventory.NewRegistration(externalID, "https://simulator.example", externalID, inventory.SyntheticContractV1, "synthetic-test:"+externalID, inventory.SyntheticTransportPolicyV1, map[string]string{"site": site, "rack": rack, "family": "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := persistence.RegisterTarget(ctx, pool, id, registration); err != nil {
		t.Fatal(err)
	}
}
