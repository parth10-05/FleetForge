package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/orchestration"
)

var wakeupUUIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// WakeupWorkAdapter binds the generic worker contract to short PostgreSQL
// transactions. Token generation is injected so operational randomness stays
// outside the fencing transaction and tests remain deterministic.
type WakeupWorkAdapter struct {
	Pool          *pgxpool.Pool
	Owner         string
	LeaseDuration time.Duration
	Now           func() time.Time
	NewToken      func() (string, error)
}

func (a WakeupWorkAdapter) Claim(ctx context.Context, envelope orchestration.Envelope) (orchestration.ClaimedWork, orchestration.ClaimDisposition, error) {
	if a.Pool == nil || a.Owner == "" || a.LeaseDuration <= 0 || a.Now == nil || a.NewToken == nil {
		return orchestration.ClaimedWork{}, orchestration.ClaimIgnored, fmt.Errorf("invalid wakeup work adapter")
	}
	now := a.Now().UTC()
	token, err := a.NewToken()
	if err != nil {
		return orchestration.ClaimedWork{}, orchestration.ClaimIgnored, fmt.Errorf("create wakeup lease token: %w", err)
	}
	claimed, owned, err := ClaimDueWorkForWakeup(ctx, a.Pool, ClaimWakeupWorkRequest{Owner: a.Owner, LeaseToken: token, DueWorkID: envelope.DueWorkID, CommandID: envelope.CommandID, DeliveryGeneration: envelope.DeliveryGeneration, ExpectedRevision: envelope.ExpectedRevision, ExpectedCommandEpoch: envelope.ExpectedCommandEpoch, Now: now, LeaseExpiresAt: now.Add(a.LeaseDuration)})
	if err != nil {
		return orchestration.ClaimedWork{}, orchestration.ClaimIgnored, err
	}
	if !owned {
		return orchestration.ClaimedWork{}, orchestration.ClaimIgnored, nil
	}
	return orchestration.ClaimedWork{DueWorkID: claimed.ID, CommandID: claimed.CommandID, LeaseToken: claimed.LeaseToken, WorkType: claimed.WorkType, DeliveryGeneration: claimed.DeliveryGeneration, ExpectedRevision: claimed.ExpectedRevision, ExpectedCommandEpoch: claimed.ExpectedCommandEpoch}, orchestration.ClaimOwned, nil
}

func (a WakeupWorkAdapter) Complete(ctx context.Context, work orchestration.ClaimedWork) error {
	if a.Pool == nil || a.Now == nil || work.DueWorkID == "" || work.CommandID == "" || work.LeaseToken == "" || work.ExpectedRevision < 0 || work.DeliveryGeneration < 0 {
		return fmt.Errorf("invalid wakeup completion")
	}
	now := a.Now().UTC()
	tx, err := a.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin wakeup completion: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tag, err := tx.Exec(ctx, `UPDATE due_work SET delivery_state='COMPLETED',lease_token=NULL,lease_owner=NULL,lease_expires_at=NULL,updated_at=$5 WHERE id=$1::uuid AND command_id=$2::uuid AND delivery_generation=$3 AND delivery_state='CLAIMED' AND lease_token=$4::uuid AND expected_revision=$6 AND expected_command_epoch IS NOT DISTINCT FROM $7`, work.DueWorkID, work.CommandID, work.DeliveryGeneration, work.LeaseToken, now, work.ExpectedRevision, work.ExpectedCommandEpoch)
	if err != nil {
		return classifyDueWorkError(err, "complete wakeup work")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: stale wakeup completion", ErrLeaseConflict)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO wakeup_execution_evidence (id,due_work_id,command_id,delivery_generation,work_type,completed_at) VALUES (md5($1)::uuid,$2::uuid,$3::uuid,$4,$5,$6)`, "fleetforge.wakeup-complete/v1:"+work.DueWorkID+":"+fmt.Sprint(work.DeliveryGeneration), work.DueWorkID, work.CommandID, work.DeliveryGeneration, work.WorkType, now); err != nil {
		return fmt.Errorf("record wakeup completion evidence: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return classifyDueWorkError(err, "commit wakeup completion")
	}
	return nil
}

func (a WakeupWorkAdapter) Quarantine(ctx context.Context, receipt orchestration.Receipt, reason string) error {
	if a.Pool == nil || a.Now == nil || !validWakeupQuarantineReason(reason) {
		return fmt.Errorf("invalid wakeup quarantine")
	}
	key := fmt.Sprintf("%x", sha256.Sum256(append([]byte(reason+":"), receipt.Raw...)))
	metadata := wakeupReceiptMetadata(receipt.Raw)
	now := a.Now().UTC()
	_, err := a.Pool.Exec(ctx, `INSERT INTO wakeup_quarantine (id,deduplication_key,observed_schema_version,observed_queue_class,due_work_id,command_id,reason_code,first_observed_at,last_observed_at) VALUES (md5($1)::uuid,$2,$3,$4,$5::uuid,$6::uuid,$7,$8,$8) ON CONFLICT (deduplication_key) DO UPDATE SET last_observed_at=EXCLUDED.last_observed_at,occurrences=wakeup_quarantine.occurrences+1`, "fleetforge.wakeup-quarantine/v1:"+key, key, metadata.schemaVersion, metadata.queueClass, nullableWakeupUUID(metadata.dueWorkID), nullableWakeupUUID(metadata.commandID), reason, now)
	if err != nil {
		return fmt.Errorf("record wakeup quarantine: %w", err)
	}
	return nil
}

// nullableWakeupUUID ensures malformed input stays metadata-only. PostgreSQL
// must receive NULL for an unavailable identifier; an empty string cannot be
// cast to uuid and must never make the durable quarantine write fail.
func nullableWakeupUUID(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func validWakeupQuarantineReason(reason string) bool {
	switch reason {
	case orchestration.QuarantineOversize, orchestration.QuarantineMalformedEncoding, orchestration.QuarantineUnknownVersion, orchestration.QuarantineInvalidIdentifier, orchestration.QuarantineInvalidQueueClass, orchestration.QuarantineInconsistentFencing, orchestration.QuarantineUnsupportedWorkType, orchestration.QuarantinePermanentPublish:
		return true
	}
	return false
}

type wakeupMetadata struct{ schemaVersion, queueClass, dueWorkID, commandID string }

// wakeupReceiptMetadata retains only safely parsed, bounded routing fields.
// It never trusts them for dispatch, linkage, or state mutation.
func wakeupReceiptMetadata(raw []byte) wakeupMetadata {
	var fields struct {
		SchemaVersion string `json:"schema_version"`
		QueueClass    string `json:"queue_class"`
		DueWorkID     string `json:"due_work_id"`
		CommandID     string `json:"command_id"`
	}
	if len(raw) > orchestration.MaxEnvelopeBytes || json.Unmarshal(raw, &fields) != nil {
		return wakeupMetadata{}
	}
	metadata := wakeupMetadata{}
	if len(fields.SchemaVersion) <= 64 {
		metadata.schemaVersion = fields.SchemaVersion
	}
	if len(fields.QueueClass) <= 32 {
		metadata.queueClass = fields.QueueClass
	}
	if wakeupUUIDPattern.MatchString(fields.DueWorkID) {
		metadata.dueWorkID = fields.DueWorkID
	}
	if wakeupUUIDPattern.MatchString(fields.CommandID) {
		metadata.commandID = fields.CommandID
	}
	return metadata
}
