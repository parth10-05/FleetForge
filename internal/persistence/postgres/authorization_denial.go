package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthorizationDenial is intentionally restricted to safe, post-verification
// identity and request classification facts. It must never carry tokens,
// headers, claims, raw idempotency keys, or request bodies.
type AuthorizationDenial struct {
	ID, ActorSubject, Route, Action, ReasonCode, CorrelationID string
}

func RecordAuthorizationDenial(ctx context.Context, pool *pgxpool.Pool, denial AuthorizationDenial) error {
	if pool == nil || !validAuthorizationDenial(denial) {
		return fmt.Errorf("invalid authorization denial")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO authorization_denials (id, actor_subject, route, action, reason_code, correlation_id) VALUES ($1::uuid,$2,$3,$4,$5,$6::uuid)`, denial.ID, denial.ActorSubject, denial.Route, denial.Action, denial.ReasonCode, denial.CorrelationID); err != nil {
		return fmt.Errorf("insert authorization denial: %w", err)
	}
	return nil
}

func validAuthorizationDenial(denial AuthorizationDenial) bool {
	return validReferenceUUID(denial.ID) && validReferenceUUID(denial.CorrelationID) && len(denial.ActorSubject) > 0 && len(denial.ActorSubject) <= 256 && validRoute(denial.Route) && validReferenceToken(denial.Action) && validReferenceToken(denial.ReasonCode)
}

func validRoute(route string) bool {
	if len(route) == 0 || len(route) > 256 || route[0] != '/' {
		return false
	}
	for _, character := range route {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '/' || character == '-' || character == '_' || character == '.' || character == ':') {
			return false
		}
	}
	return true
}
