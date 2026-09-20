package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/domain/reason"
)

// ReplayRecord is the durable, typed decision evidence retained independently
// from device observations. It is intentionally persistence-facing.
type ReplayRecord struct {
	ID, AggregateType, AggregateID, ReducerVersion, PolicyVersion, Result string
	AggregateRevision, DecisionSequence                                   int64
	LogicalTime                                                           time.Time
	NormalizedFacts                                                       json.RawMessage
	Reason                                                                reason.Code
}

// LoadReplayRecords reads only decision_inputs. It never consults current
// aggregate state or device_observations, so observation retention cannot
// change a historical decision result.
func LoadReplayRecords(ctx context.Context, pool *pgxpool.Pool, aggregateType, aggregateID string) ([]ReplayRecord, error) {
	if aggregateType != "CAMPAIGN" && aggregateType != "TARGET_UPDATE" || aggregateID == "" {
		return nil, fmt.Errorf("%w: invalid replay aggregate", ErrCorruptPersistedValue)
	}
	rows, err := pool.Query(ctx, `SELECT id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code FROM decision_inputs WHERE aggregate_type=$1 AND aggregate_id=$2::uuid ORDER BY aggregate_revision,decision_sequence`, aggregateType, aggregateID)
	if err != nil {
		return nil, fmt.Errorf("load replay records: %w", err)
	}
	defer rows.Close()
	items := []ReplayRecord{}
	for rows.Next() {
		var item ReplayRecord
		var code string
		if err := rows.Scan(&item.ID, &item.AggregateType, &item.AggregateID, &item.AggregateRevision, &item.DecisionSequence, &item.LogicalTime, &item.ReducerVersion, &item.PolicyVersion, &item.NormalizedFacts, &item.Result, &code); err != nil {
			return nil, fmt.Errorf("scan replay record: %w", err)
		}
		parsed, err := reason.ParseCode(code)
		if err != nil {
			return nil, fmt.Errorf("%w: replay reason %q", ErrCorruptPersistedValue, code)
		}
		item.Reason = parsed
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read replay records: %w", err)
	}
	if err := ValidateReplayRecords(items); err != nil {
		return nil, err
	}
	return items, nil
}

func ValidateReplayRecords(records []ReplayRecord) error {
	if len(records) == 0 {
		return fmt.Errorf("%w: replay has no decision inputs", ErrCorruptPersistedValue)
	}
	lastRevision := int64(-1)
	lastSequence := int64(-1)
	for _, record := range records {
		if (record.AggregateType != "CAMPAIGN" && record.AggregateType != "TARGET_UPDATE") || record.AggregateID == "" || record.AggregateRevision < 0 || record.DecisionSequence < 0 || record.LogicalTime.IsZero() || record.ReducerVersion == "" || record.PolicyVersion == "" || record.Result == "" {
			return fmt.Errorf("%w: invalid replay record", ErrCorruptPersistedValue)
		}
		if _, err := reason.ParseCode(record.Reason.String()); err != nil {
			return fmt.Errorf("%w: unknown replay reason", ErrCorruptPersistedValue)
		}
		var facts map[string]json.RawMessage
		if len(record.NormalizedFacts) == 0 || json.Unmarshal(record.NormalizedFacts, &facts) != nil || facts == nil {
			return fmt.Errorf("%w: malformed replay facts", ErrCorruptPersistedValue)
		}
		if record.AggregateRevision < lastRevision || (record.AggregateRevision == lastRevision && record.DecisionSequence != lastSequence+1) {
			return fmt.Errorf("%w: replay sequence gap", ErrCorruptPersistedValue)
		}
		if record.AggregateRevision != lastRevision {
			lastSequence = -1
		}
		lastRevision = record.AggregateRevision
		lastSequence = record.DecisionSequence
	}
	return nil
}
