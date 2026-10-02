package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
	"github.com/parth10-05/fleetforge/internal/domain/value"
	"github.com/parth10-05/fleetforge/internal/persistence/postgres/sqlcgen"
)

const maxSelectedTargets = 100

var ErrSelectorResultLimit = errors.New("selector result exceeds configured maximum")

// SelectedTarget is the live, bounded input used to construct an immutable
// preflight candidate. It is never used to re-evaluate a stored candidate.
type SelectedTarget struct {
	ID       value.TargetID
	Revision value.Revision
	Labels   value.FailureDomainLabels
	Family   value.LabelValue
}

// SelectTargets evaluates the bounded selector through one fixed,
// parameterized query. It asks PostgreSQL for cap+1 rows so overflow fails
// closed instead of silently truncating campaign membership.
func SelectTargets(ctx context.Context, pool *pgxpool.Pool, selector inventory.Selector, maximum int) ([]SelectedTarget, error) {
	if maximum <= 0 || maximum > maxSelectedTargets || len(selector.Terms()) == 0 {
		return nil, fmt.Errorf("invalid target selection request")
	}
	keys := make([]string, 0, len(selector.Terms()))
	values := make([]string, 0, len(selector.Terms()))
	for _, term := range selector.Terms() {
		keys = append(keys, term.Key.String())
		values = append(values, term.Value.String())
	}
	rows, err := sqlcgen.New(pool).SelectTargetsByLabels(ctx, sqlcgen.SelectTargetsByLabelsParams{Column1: keys, Column2: values, Limit: int32(maximum + 1)})
	if err != nil {
		return nil, fmt.Errorf("select targets by labels: %w", err)
	}
	if len(rows) > maximum {
		return nil, ErrSelectorResultLimit
	}
	result := make([]SelectedTarget, 0, len(rows))
	for _, row := range rows {
		id, err := value.ParseTargetID(row.ID.String())
		if err != nil {
			return nil, fmt.Errorf("map selected target ID: %w", err)
		}
		if row.Revision < 0 {
			return nil, fmt.Errorf("map selected target revision: negative")
		}
		labels, err := value.NewFailureDomainLabels(map[string]string{"site": row.SiteKey, "rack": row.RackKey, "family": row.Family})
		if err != nil {
			return nil, fmt.Errorf("map selected target labels: %w", err)
		}
		family, err := value.ParseLabelValue(row.Family)
		if err != nil {
			return nil, fmt.Errorf("map selected target family: %w", err)
		}
		result = append(result, SelectedTarget{ID: id, Revision: value.Revision(row.Revision), Labels: labels, Family: family})
	}
	return result, nil
}
