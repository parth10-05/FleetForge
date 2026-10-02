package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/parth10-05/fleetforge/internal/application/inventory"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
)

// RegistrationAdapter composes the transport-neutral inventory application
// command with the serializable PostgreSQL registration writer.
type RegistrationAdapter struct{ Pool *pgxpool.Pool }

func (adapter RegistrationAdapter) Register(ctx context.Context, targetID string, registration inventory.Registration) (app.RegistrationReceipt, error) {
	if adapter.Pool == nil {
		return app.RegistrationReceipt{}, fmt.Errorf("inventory persistence is unavailable")
	}
	result, err := RegisterTarget(ctx, adapter.Pool, targetID, registration)
	if err != nil {
		return app.RegistrationReceipt{}, err
	}
	return app.RegistrationReceipt{TargetID: result.TargetID, Created: result.Created}, nil
}
