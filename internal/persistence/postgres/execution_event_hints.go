package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/eventhint"
)

type EventHintStore struct{ Pool *pgxpool.Pool }

func (store EventHintStore) RecordEventHint(ctx context.Context, hint eventhint.Hint) (string, error) {
	return RecordEventHint(ctx, store.Pool, hint)
}

func (store EventHintStore) QuarantineEventHint(ctx context.Context, body []byte, reason string, receivedAt time.Time) error {
	return QuarantineEventHint(ctx, store.Pool, body, reason, receivedAt)
}

// QuarantineEventHint retains only a bounded classification and payload digest
// for an unauthenticated or malformed callback. It deliberately accepts no
// raw body, headers, target identity, or scheduling authority.
func QuarantineEventHint(ctx context.Context, pool *pgxpool.Pool, body []byte, reason string, receivedAt time.Time) error {
	if pool == nil || len(body) == 0 || len(body) > 8192 || len(reason) == 0 || len(reason) > 64 || receivedAt.IsZero() {
		return fmt.Errorf("invalid event hint quarantine")
	}
	digest := sha256.Sum256(body)
	identity := "fleetforge.execution.event-quarantine/v1:" + hex.EncodeToString(digest[:]) + ":" + reason
	_, err := pool.Exec(ctx, `INSERT INTO execution_event_quarantine (id,source,payload_digest,reason_code,received_at)
VALUES (md5($1)::uuid,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, identity, eventhint.Source, "sha256:"+hex.EncodeToString(digest[:]), reason, receivedAt.UTC())
	if err != nil {
		return fmt.Errorf("insert event quarantine: %w", err)
	}
	return nil
}

// RecordEventHint stores one authenticated advisory receipt. It may create a
// fenced TASK_OBSERVE wakeup for the matching accepted attempt; it never
// updates target state, attempt status, reservations, or submission work.
func RecordEventHint(ctx context.Context, pool *pgxpool.Pool, hint eventhint.Hint) (string, error) {
	if pool == nil || hint.Source != eventhint.Source || hint.EventID == "" || hint.TargetKey == "" || hint.TaskID == "" || hint.Sequence <= 0 || hint.ObservedAt.IsZero() || hint.ReceivedAt.IsZero() || hint.PayloadDigest == "" {
		return "", fmt.Errorf("invalid execution event hint")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return "", fmt.Errorf("begin event hint: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var existing string
	err = tx.QueryRow(ctx, `SELECT disposition FROM execution_event_receipts WHERE source=$1 AND event_id=$2`, hint.Source, hint.EventID).Scan(&existing)
	if err == nil {
		_ = tx.Commit(ctx)
		return "DUPLICATE", nil
	}
	if err != pgx.ErrNoRows {
		return "", fmt.Errorf("read event receipt: %w", err)
	}
	var head int64
	err = tx.QueryRow(ctx, `SELECT sequence FROM execution_event_heads WHERE target_key=$1 FOR UPDATE`, hint.TargetKey).Scan(&head)
	if err != nil && err != pgx.ErrNoRows {
		return "", fmt.Errorf("lock event head: %w", err)
	}
	disposition := "UNMATCHED"
	var updateID string
	var revision, epoch int64
	if err == nil && hint.Sequence <= head {
		disposition = "LATE"
	} else {
		matchErr := tx.QueryRow(ctx, `SELECT tu.id::text,tu.revision,tu.command_epoch
FROM targets t JOIN firmware_update_attempts a ON a.target_id=t.id
JOIN target_updates tu ON tu.id=a.target_update_id
WHERE t.target_key=$1 AND a.remote_task_reference=$2 AND a.status='ACCEPTED' AND tu.state='UPDATE_ACCEPTED'
FOR SHARE OF t,a,tu`, hint.TargetKey, "/redfish/v1/TaskService/Tasks/"+hint.TaskID).Scan(&updateID, &revision, &epoch)
		if matchErr == nil {
			disposition = "SCHEDULED"
		} else if matchErr != pgx.ErrNoRows {
			return "", fmt.Errorf("match event hint: %w", matchErr)
		}
	}
	receiptID := "fleetforge.execution.event-receipt/v1:" + hint.Source + ":" + hint.EventID
	if _, err = tx.Exec(ctx, `INSERT INTO execution_event_receipts (id,source,event_id,target_key,task_id,sequence,observed_at,received_at,payload_digest,disposition,target_update_id)
VALUES (md5($1)::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,'')::uuid)`, receiptID, hint.Source, hint.EventID, hint.TargetKey, hint.TaskID, hint.Sequence, hint.ObservedAt.UTC(), hint.ReceivedAt.UTC(), hint.PayloadDigest, disposition, updateID); err != nil {
		return "", fmt.Errorf("insert event receipt: %w", err)
	}
	if disposition == "SCHEDULED" {
		if _, err = tx.Exec(ctx, `INSERT INTO execution_event_heads (target_key,sequence,receipt_id,updated_at) VALUES ($1,$2,md5($3)::uuid,$4)
ON CONFLICT (target_key) DO UPDATE SET sequence=EXCLUDED.sequence,receipt_id=EXCLUDED.receipt_id,updated_at=EXCLUDED.updated_at
WHERE execution_event_heads.sequence < EXCLUDED.sequence`, hint.TargetKey, hint.Sequence, receiptID, hint.ReceivedAt.UTC()); err != nil {
			return "", fmt.Errorf("advance event head: %w", err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,expected_command_epoch)
VALUES (md5($1)::uuid,md5($2)::uuid,'TARGET_UPDATE',$3::uuid,'TASK_OBSERVE',1,$4,$5,$6)
ON CONFLICT DO NOTHING`, "fleetforge.execution.event-work/v1:"+updateID, "fleetforge.execution.event-command/v1:"+updateID, updateID, time.Now().UTC(), revision, epoch); err != nil {
			return "", fmt.Errorf("schedule event observation: %w", err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit event hint: %w", err)
	}
	return disposition, nil
}
