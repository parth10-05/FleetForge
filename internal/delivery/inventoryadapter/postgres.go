// Package inventoryadapter adapts PostgreSQL registration to the application port.
package inventoryadapter

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	application "github.com/parth10-05/fleetforge/internal/application/inventory"
	domain "github.com/parth10-05/fleetforge/internal/domain/inventory"
	persistence "github.com/parth10-05/fleetforge/internal/persistence/postgres"
)

type PostgresRegistrationWriter struct{ Pool *pgxpool.Pool }

func (writer PostgresRegistrationWriter) Register(ctx context.Context, targetID string, registration domain.Registration) (application.RegistrationReceipt, error) {
	result, err := persistence.RegisterTarget(ctx, writer.Pool, targetID, registration)
	if err != nil {
		return application.RegistrationReceipt{}, err
	}
	return application.RegistrationReceipt{TargetID: result.TargetID, Created: result.Created}, nil
}

var _ application.RegistrationWriter = PostgresRegistrationWriter{}
