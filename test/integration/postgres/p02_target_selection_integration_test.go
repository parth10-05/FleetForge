//go:build postgresintegration

package postgres

import (
	"context"
	"errors"
	"strings"
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

func TestP02TargetSelectionFailsClosedAtTenThousandCandidates(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	if _, err := pool.Exec(ctx, `INSERT INTO targets
    (id,external_identity,endpoint_reference,site_key,rack_key,administrative_state,target_key,contract_version,credential_provider,credential_reference,transport_policy_version,registration_hash,trust_metadata_complete)
SELECT ('00000000-0000-0000-2602-' || lpad(series::text,12,'0'))::uuid,
       'selection-scale-' || series::text,
       'https://simulator.example','west','rack-scale','ACTIVE',
       'scale-' || series::text,'synthetic-redfish-v1','synthetic-test',
       'synthetic-test:scale-' || series::text,'synthetic-redfish-v1',
       'sha256:' || repeat('a',64),true
  FROM generate_series(1,10000) AS series`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO target_labels(target_id,key,value)
SELECT target.id,label.key,label.value
  FROM targets AS target
 CROSS JOIN (VALUES ('site'::varchar,'west'::varchar),('family'::varchar,'synthetic'::varchar)) AS label(key,value)
 WHERE target.external_identity LIKE 'selection-scale-%'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE targets; ANALYZE target_labels"); err != nil {
		t.Fatal(err)
	}
	selector, err := inventory.ParseSelector([]byte(`{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"west"},{"key":"family","value":"synthetic"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := persistence.SelectTargets(ctx, pool, selector, 100)
	if !errors.Is(err, persistence.ErrSelectorResultLimit) || selected != nil {
		t.Fatalf("selection must reject cap overflow: selected=%v err=%v", selected, err)
	}
}

func TestP02TargetSelectionRetainsExactLabelLookupIndex(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	var definition string
	if err := pool.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname=current_schema() AND indexname='target_labels_exact_lookup_index'`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, "(key, value, target_id)") {
		t.Fatalf("exact-label lookup index definition=%q", definition)
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
