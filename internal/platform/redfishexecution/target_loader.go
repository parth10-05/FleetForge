package redfishexecution

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresTargetLoader reads exactly one current trusted target registration.
// It is intentionally separate from the submission transaction so no lock is
// retained while a target-side HTTP request is in flight.
type PostgresTargetLoader struct{ Pool *pgxpool.Pool }

func (loader PostgresTargetLoader) LoadExecutionTarget(ctx context.Context, targetID string) (Target, error) {
	if loader.Pool == nil || targetID == "" {
		return Target{}, fmt.Errorf("invalid execution target request")
	}
	var target Target
	err := loader.Pool.QueryRow(ctx, `SELECT endpoint_reference,target_key,credential_reference
FROM targets WHERE id=$1::uuid AND administrative_state='ACTIVE' AND trust_metadata_complete
AND contract_version='synthetic-redfish-v1' AND credential_provider='synthetic-test'`, targetID).Scan(&target.Origin, &target.TargetKey, &target.CredentialReference)
	if err == pgx.ErrNoRows {
		return Target{}, fmt.Errorf("execution target is unavailable")
	}
	if err != nil {
		return Target{}, fmt.Errorf("load execution target: %w", err)
	}
	return target, nil
}
