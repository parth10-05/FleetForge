package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrCampaignProjectionNotFound = errors.New("campaign projection not found")

type CampaignProjection struct {
	ID, State string
	Revision  int64
	AsOf      time.Time
}

func GetCampaignProjection(ctx context.Context, pool *pgxpool.Pool, campaignID string) (CampaignProjection, error) {
	if pool == nil {
		return CampaignProjection{}, fmt.Errorf("campaign projection persistence is unavailable")
	}
	if _, err := uuidParameter(campaignID); err != nil {
		return CampaignProjection{}, fmt.Errorf("invalid campaign projection ID")
	}
	var projection CampaignProjection
	if err := pool.QueryRow(ctx, "SELECT id::text,state,revision,updated_at FROM campaigns WHERE id=$1::uuid", campaignID).Scan(&projection.ID, &projection.State, &projection.Revision, &projection.AsOf); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CampaignProjection{}, ErrCampaignProjectionNotFound
		}
		return CampaignProjection{}, fmt.Errorf("load campaign projection: %w", err)
	}
	projection.AsOf = projection.AsOf.UTC()
	return projection, nil
}

func ListCampaignTargetProjections(ctx context.Context, pool *pgxpool.Pool, campaignID, afterID string, limit int) ([]TargetProjection, error) {
	if pool == nil || limit < 1 || limit > 101 {
		return nil, fmt.Errorf("invalid campaign target projection request")
	}
	if _, err := uuidParameter(campaignID); err != nil {
		return nil, fmt.Errorf("invalid campaign projection ID")
	}
	if afterID != "" {
		if _, err := uuidParameter(afterID); err != nil {
			return nil, fmt.Errorf("invalid campaign target projection cursor")
		}
	}
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM campaigns WHERE id=$1::uuid)", campaignID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check campaign projection: %w", err)
	}
	if !exists {
		return nil, ErrCampaignProjectionNotFound
	}
	rows, err := pool.Query(ctx, `SELECT t.id::text,tu.state,tu.revision,tu.updated_at
FROM campaign_targets ct JOIN targets t ON t.id=ct.target_id JOIN target_updates tu ON tu.campaign_target_id=ct.id
WHERE ct.campaign_id=$1::uuid AND (NULLIF($2,'')::uuid IS NULL OR t.id>NULLIF($2,'')::uuid)
ORDER BY t.id LIMIT $3`, campaignID, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list campaign target projections: %w", err)
	}
	defer rows.Close()
	items := make([]TargetProjection, 0, limit)
	for rows.Next() {
		var item TargetProjection
		if err := rows.Scan(&item.ID, &item.State, &item.Revision, &item.AsOf); err != nil {
			return nil, fmt.Errorf("scan campaign target projection: %w", err)
		}
		item.AsOf = item.AsOf.UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate campaign target projections: %w", err)
	}
	return items, nil
}

type CampaignAuditRecord struct {
	Action, Outcome                  string
	AggregateRevision, EventSequence int64
	OccurredAt                       time.Time
}

func ListCampaignAuditRecords(ctx context.Context, pool *pgxpool.Pool, campaignID string, afterRevision, afterSequence int64, limit int) ([]CampaignAuditRecord, error) {
	if pool == nil || afterRevision < 0 || afterSequence < -1 || limit < 1 || limit > 101 {
		return nil, fmt.Errorf("invalid campaign audit request")
	}
	if _, err := uuidParameter(campaignID); err != nil {
		return nil, fmt.Errorf("invalid campaign projection ID")
	}
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM campaigns WHERE id=$1::uuid)", campaignID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check campaign projection: %w", err)
	}
	if !exists {
		return nil, ErrCampaignProjectionNotFound
	}
	rows, err := pool.Query(ctx, `SELECT action,reason_code,aggregate_revision,event_sequence,created_at FROM audit_events
WHERE aggregate_type='CAMPAIGN' AND aggregate_id=$1::uuid AND (aggregate_revision,event_sequence) > ($2,$3)
ORDER BY aggregate_revision,event_sequence LIMIT $4`, campaignID, afterRevision, afterSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("list campaign audit records: %w", err)
	}
	defer rows.Close()
	items := make([]CampaignAuditRecord, 0, limit)
	for rows.Next() {
		var item CampaignAuditRecord
		if err := rows.Scan(&item.Action, &item.Outcome, &item.AggregateRevision, &item.EventSequence, &item.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan campaign audit record: %w", err)
		}
		item.OccurredAt = item.OccurredAt.UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate campaign audit records: %w", err)
	}
	return items, nil
}
