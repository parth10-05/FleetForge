package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/access"
	"github.com/parth10-05/fleetforge/internal/application/control"
	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

var ErrArtifactInitiationConflict = errors.New("artifact initiation conflict")

func (adapter ControlArtifactAdapter) InitiateArtifact(ctx context.Context, principal access.Principal, meta control.CommandMeta, command control.InitiateArtifactCommand) (control.ArtifactInitiationResult, error) {
	if adapter.Pool == nil {
		return control.ArtifactInitiationResult{}, fmt.Errorf("artifact initiation persistence is unavailable")
	}
	claim := IdempotencyClaim{ID: meta.ClaimID, ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, RequestBody: meta.CanonicalRequest, CommandID: meta.CommandID, OwnerToken: meta.OwnerToken, TTL: 30 * time.Minute}
	if err := ClaimIdempotencyKey(ctx, adapter.Pool, claim); err != nil {
		if !errors.Is(err, ErrIdempotencyInProgress) {
			return control.ArtifactInitiationResult{}, err
		}
		_, reference, found, replayErr := GetCompletedIdempotencyRecord(ctx, adapter.Pool, principal.Subject, meta.Route, meta.IdempotencyKey)
		if replayErr != nil || !found {
			if replayErr != nil {
				return control.ArtifactInitiationResult{}, replayErr
			}
			return control.ArtifactInitiationResult{}, ErrIdempotencyInProgress
		}
		return resolveArtifactInitiationReplay(ctx, adapter.Pool, reference)
	}
	reference, err := (ResponseReference{Operation: "artifact-initiate", ResourceID: command.ArtifactID, Revision: 0, CommandID: meta.CommandID}).Encode()
	if err != nil {
		return control.ArtifactInitiationResult{}, fmt.Errorf("encode artifact initiation response reference: %w", err)
	}
	input := IdempotentArtifactInitiation{ArtifactID: command.ArtifactID, GrantID: command.GrantID, FirmwareVersion: command.FirmwareVersion, Metadata: command.Metadata, ExpiresAt: command.ExpiresAt, CommandID: meta.CommandID, CorrelationID: meta.CorrelationID, OwnerToken: meta.OwnerToken, OutboxEventID: meta.OutboxEventID, DecisionInputID: meta.DecisionInputID, Completion: IdempotencyCompletion{ActorID: principal.Subject, Route: meta.Route, RawKey: meta.IdempotencyKey, ResponseStatus: 201, ResponseReference: reference}}
	if err := ApplyIdempotentArtifactInitiation(ctx, adapter.Pool, input); err != nil {
		return control.ArtifactInitiationResult{}, err
	}
	return resolveArtifactInitiationReplay(ctx, adapter.Pool, reference)
}

type IdempotentArtifactInitiation struct {
	ArtifactID, GrantID, FirmwareVersion string
	Metadata                             artifact.UploadMetadata
	ExpiresAt                            time.Time
	CommandID, CorrelationID, OwnerToken string
	OutboxEventID, DecisionInputID       string
	Completion                           IdempotencyCompletion
}

func ApplyIdempotentArtifactInitiation(ctx context.Context, pool *pgxpool.Pool, input IdempotentArtifactInitiation) error {
	if err := validateArtifactInitiation(input); err != nil {
		return err
	}
	artifactID, _ := value.ParseArtifactID(input.ArtifactID)
	key, err := artifact.UploadObjectKey(artifactID)
	if err != nil {
		return fmt.Errorf("derive artifact upload key: %w", err)
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin idempotent artifact initiation: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	keyHash := NormalizeKey(input.Completion.ActorID, input.Completion.Route, input.Completion.RawKey)
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM idempotency_records WHERE actor_id=$1 AND route=$2 AND key_hash=$3 AND owner_token=$4::uuid FOR UPDATE", input.Completion.ActorID, input.Completion.Route, keyHash, input.OwnerToken).Scan(&state); err != nil {
		return fmt.Errorf("lock artifact idempotency claim: %w", err)
	}
	if state != string(IdempotencyStateInProgress) {
		return fmt.Errorf("%w: claim is %s", ErrIdempotencyConflict, state)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO firmware_artifacts (id,firmware_version,sha256_digest,object_reference,upload_grant_id,upload_grant_expires_at,upload_size_bytes,upload_media_type,state) VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,'UPLOADING')", input.ArtifactID, input.FirmwareVersion, input.Metadata.Digest().String(), key, input.GrantID, input.ExpiresAt.UTC(), input.Metadata.Size(), input.Metadata.MediaType()); err != nil {
		return fmt.Errorf("%w: create artifact: %v", ErrArtifactInitiationConflict, err)
	}
	facts, err := json.Marshal(map[string]any{"artifact_id": input.ArtifactID, "grant_id": input.GrantID, "expires_at": input.ExpiresAt.UTC().Format(time.RFC3339Nano), "sha256": input.Metadata.Digest().String(), "size_bytes": input.Metadata.Size()})
	if err != nil {
		return fmt.Errorf("encode artifact initiation facts: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO audit_events (aggregate_type,aggregate_id,aggregate_revision,event_sequence,actor_id,command_id,correlation_id,causation_id,action,reason_code) VALUES ('ARTIFACT',$1::uuid,0,0,$2,$3::uuid,$4::uuid,$3::uuid,'ARTIFACT_UPLOAD_INITIATED','artifact.upload.initiated')", input.ArtifactID, input.Completion.ActorID, input.CommandID, input.CorrelationID); err != nil {
		return fmt.Errorf("append artifact initiation audit: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO decision_inputs (id,aggregate_type,aggregate_id,aggregate_revision,decision_sequence,logical_time,reducer_version,policy_version,normalized_facts,result,reason_code) VALUES ($1::uuid,'ARTIFACT',$2::uuid,0,0,now(),'control-api/v1','artifact-upload/v1',$3::jsonb,'UPLOADING','artifact.upload.initiated')", input.DecisionInputID, input.ArtifactID, string(facts)); err != nil {
		return fmt.Errorf("append artifact initiation decision: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO outbox_events (event_id,aggregate_type,aggregate_id,aggregate_revision,event_type,schema_version,payload) VALUES ($1::uuid,'ARTIFACT',$2::uuid,0,'artifact.upload_initiated',1,$3::jsonb)", input.OutboxEventID, input.ArtifactID, string(facts)); err != nil {
		return fmt.Errorf("append artifact initiation outbox: %w", err)
	}
	if err := completeIdempotencyKeyInTx(ctx, tx, input.Completion, input.OwnerToken, keyHash); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit idempotent artifact initiation: %w", err)
	}
	return nil
}

func validateArtifactInitiation(input IdempotentArtifactInitiation) error {
	reference, err := DecodeResponseReference(input.Completion.ResponseReference)
	if err != nil || reference.Operation != "artifact-initiate" || reference.ResourceID != input.ArtifactID || reference.CommandID != input.CommandID || reference.Revision != 0 || input.Completion.ResponseStatus != 201 || input.FirmwareVersion == "" || input.GrantID == "" || input.ExpiresAt.IsZero() {
		return fmt.Errorf("invalid idempotent artifact initiation")
	}
	for _, raw := range []string{input.ArtifactID, input.CommandID, input.CorrelationID, input.OwnerToken, input.OutboxEventID, input.DecisionInputID} {
		if _, err := uuidParameter(raw); err != nil {
			return fmt.Errorf("invalid idempotent artifact initiation")
		}
	}
	if _, err := value.ParseArtifactID(input.ArtifactID); err != nil {
		return fmt.Errorf("invalid idempotent artifact initiation")
	}
	return nil
}

func resolveArtifactInitiationReplay(ctx context.Context, pool *pgxpool.Pool, responseReference string) (control.ArtifactInitiationResult, error) {
	reference, err := DecodeResponseReference(responseReference)
	if err != nil || reference.Operation != "artifact-initiate" {
		return control.ArtifactInitiationResult{}, fmt.Errorf("invalid artifact initiation response reference")
	}
	var firmwareVersion, digestRaw, key, grantID, mediaType string
	var size int64
	var acceptedAt, expiresAt time.Time
	if err := pool.QueryRow(ctx, "SELECT firmware_version,sha256_digest,object_reference,upload_grant_id,upload_grant_expires_at,upload_size_bytes,upload_media_type,created_at FROM firmware_artifacts WHERE id=$1::uuid", reference.ResourceID).Scan(&firmwareVersion, &digestRaw, &key, &grantID, &expiresAt, &size, &mediaType, &acceptedAt); err != nil {
		return control.ArtifactInitiationResult{}, fmt.Errorf("read artifact initiation response: %w", err)
	}
	digest, err := value.ParseSHA256Digest(digestRaw)
	if err != nil {
		return control.ArtifactInitiationResult{}, fmt.Errorf("map artifact initiation digest: %w", err)
	}
	metadata, err := artifact.NewUploadMetadata("firmware.bin", mediaType, size, digest)
	if err != nil {
		return control.ArtifactInitiationResult{}, fmt.Errorf("map artifact initiation metadata: %w", err)
	}
	return control.ArtifactInitiationResult{ArtifactID: reference.ResourceID, CommandID: reference.CommandID, GrantID: grantID, Key: key, AcceptedAt: acceptedAt.UTC(), ExpiresAt: expiresAt.UTC(), Metadata: metadata}, nil
}

var _ control.ArtifactInitiationStore = ControlArtifactAdapter{}
