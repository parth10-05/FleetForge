package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parth10-05/fleetforge/internal/application/execution"
)

// CommitReconciliation persists a journal-derived ambiguity decision. It
// cannot manufacture retry permission: PROVEN_ABSENT is accepted only when
// both independent absence and late-caller exclusion are explicitly present.
func CommitReconciliation(ctx context.Context, pool *pgxpool.Pool, work execution.WorkFence, decision execution.ReconciliationDecision, now time.Time) error {
	if pool == nil || now.IsZero() || work.DueWorkID == "" || work.CommandID == "" || work.LeaseToken == "" || work.ExpectedRevision < 0 || work.DeliveryGeneration < 0 || decision.Work != work || !validReconciliationDecision(decision) {
		return fmt.Errorf("invalid reconciliation decision")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin reconciliation: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var attemptID, updateID, state string
	var epoch, revision int64
	err = tx.QueryRow(ctx, `SELECT a.id::text,a.target_update_id::text,tu.state,a.captured_command_epoch,tu.revision
FROM due_work d JOIN submission_admissions s ON s.target_update_id=d.subject_id
JOIN firmware_update_attempts a ON a.id=s.active_attempt_id JOIN target_updates tu ON tu.id=d.subject_id
WHERE d.id=$1::uuid AND d.command_id=$2::uuid AND d.delivery_generation=$3 AND d.delivery_state='CLAIMED'
AND d.lease_token=$4::uuid AND d.expected_revision=$5 AND d.expected_command_epoch IS NOT DISTINCT FROM $6
AND d.work_type='AMBIGUITY_RECONCILE' AND a.status='AMBIGUOUS' AND tu.state='NEEDS_ATTENTION'
FOR UPDATE OF d,s,a,tu`, work.DueWorkID, work.CommandID, work.DeliveryGeneration, work.LeaseToken, work.ExpectedRevision, work.ExpectedCommandEpoch).Scan(&attemptID, &updateID, &state, &epoch, &revision)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("%w: reconciliation work no longer matches its fence", ErrLeaseConflict)
	}
	if err != nil {
		return classifyDueWorkError(err, "lock reconciliation")
	}
	if state != "NEEDS_ATTENTION" || revision != work.ExpectedRevision || (work.ExpectedCommandEpoch != nil && epoch != *work.ExpectedCommandEpoch) {
		return fmt.Errorf("%w: stale reconciliation", ErrLeaseConflict)
	}
	evidence, _ := json.Marshal(map[string]any{"outcome": decision.Outcome, "reason_code": decision.ReasonCode, "journal_run_id": decision.JournalRunID, "checkpoint_hash": decision.CheckpointHash, "sequence_start": decision.JournalSequenceStart, "sequence_end": decision.JournalSequenceEnd})
	if _, err = tx.Exec(ctx, `INSERT INTO ambiguity_resolutions (id,attempt_id,target_update_revision,outcome,evidence_summary,evidence_fingerprint,journal_run_id,journal_checkpoint_hash,journal_sequence_start,journal_sequence_end,journal_confirmed_absent,late_caller_excluded,late_caller_exclusion_method,proof_reference,actor_id,command_id,reason_code)
VALUES (md5($1)::uuid,$2::uuid,$3,$4,$5::jsonb,$6,$7,$8,$9,$10,$11,$12,$13,$14,'system:reconciliation',$15::uuid,$16)`, "fleetforge.ambiguity-resolution/v1:"+attemptID+":"+decision.Outcome+":"+decision.EvidenceFingerprint, attemptID, revision, resolutionOutcome(decision.Outcome), evidence, decision.EvidenceFingerprint, nullableExecutionString(decision.JournalRunID), nullableExecutionString(decision.CheckpointHash), nullableInt64(decision.JournalSequenceStart), nullableInt64(decision.JournalSequenceEnd), decision.ProvenAbsent, decision.LateCallerExcluded, nil, nil, work.CommandID, decision.ReasonCode); err != nil {
		return fmt.Errorf("persist ambiguity resolution: %w", err)
	}
	nextState, nextAttemptStatus, nextWork, resume := reconciliationTransition(decision.Outcome)
	if decision.Outcome == "PROVEN_ABSENT" {
		if _, err = tx.Exec(ctx, `UPDATE submission_admissions
SET retry_authorizations=retry_authorizations+1
WHERE id=(SELECT submission_admission_id FROM firmware_update_attempts WHERE id=$1::uuid)`, attemptID); err != nil {
			return fmt.Errorf("authorize reconciled retry: %w", err)
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE firmware_update_attempts SET status=$2,resolved_at=$3 WHERE id=$1::uuid AND status='AMBIGUOUS'`, attemptID, nextAttemptStatus, now.UTC()); err != nil {
		return fmt.Errorf("advance ambiguous attempt: %w", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE target_updates SET state=$2,resume_step=$3,revision=revision+1,updated_at=$4 WHERE id=$1::uuid AND revision=$5 AND command_epoch=$6`, updateID, nextState, nullableExecutionString(resume), now.UTC(), revision, epoch)
	if err != nil {
		return fmt.Errorf("advance reconciliation lifecycle: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: target update fence changed", ErrLeaseConflict)
	}
	tag, err = tx.Exec(ctx, `UPDATE due_work SET delivery_state='COMPLETED',lease_token=NULL,lease_owner=NULL,lease_expires_at=NULL,updated_at=$5 WHERE id=$1::uuid AND command_id=$2::uuid AND delivery_generation=$3 AND delivery_state='CLAIMED' AND lease_token=$4::uuid`, work.DueWorkID, work.CommandID, work.DeliveryGeneration, work.LeaseToken, now.UTC())
	if err != nil {
		return classifyDueWorkError(err, "complete reconciliation work")
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: reconciliation work no longer claimed", ErrLeaseConflict)
	}
	if nextWork != "" {
		if _, err = tx.Exec(ctx, `INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,expected_command_epoch)
VALUES (md5($1)::uuid,md5($2)::uuid,'TARGET_UPDATE',$3::uuid,$4,1,$5,$6,$7)`, "fleetforge.execution.work/v1:"+attemptID+":"+nextWork, "fleetforge.execution.command/v1:"+attemptID+":"+nextWork, updateID, nextWork, now.UTC(), revision+1, epoch); err != nil {
			return fmt.Errorf("schedule reconciled observation: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return classifyDueWorkError(err, "commit reconciliation")
	}
	return nil
}

func (adapter ExecutionAdapter) CommitReconciliation(ctx context.Context, work execution.WorkFence, decision execution.ReconciliationDecision) error {
	now := time.Now().UTC()
	if adapter.Now != nil {
		now = adapter.Now().UTC()
	}
	return CommitReconciliation(ctx, adapter.Pool, work, decision, now)
}

// ResolveAttention applies a verified administrator's immutable decision. It
// intentionally does not call a target or create SUBMIT work: a retry proof
// only authorizes a later fresh admission with a child attempt.
func (adapter ExecutionAdapter) ResolveAttention(ctx context.Context, command execution.AdminResolutionCommand) (execution.ResolutionResult, error) {
	if adapter.Pool == nil || !validResolutionPrincipal(command.Principal, command.Now) || command.AttemptID == "" || command.Work.CommandID == "" || command.Work.ExpectedRevision < 0 || command.EvidenceFingerprint == "" || command.Now.IsZero() {
		return execution.ResolutionResult{}, fmt.Errorf("invalid attention resolution")
	}
	tx, err := adapter.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return execution.ResolutionResult{}, fmt.Errorf("begin attention resolution: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var updateID, admissionID, state, status string
	var revision, epoch int64
	err = tx.QueryRow(ctx, `SELECT a.target_update_id::text,a.submission_admission_id::text,tu.state,a.status,tu.revision,a.captured_command_epoch
FROM firmware_update_attempts a JOIN target_updates tu ON tu.id=a.target_update_id
WHERE a.id=$1::uuid FOR UPDATE OF a,tu`, command.AttemptID).Scan(&updateID, &admissionID, &state, &status, &revision, &epoch)
	if err == pgx.ErrNoRows {
		return execution.ResolutionResult{}, fmt.Errorf("%w: unknown attention attempt", ErrLeaseConflict)
	}
	if err != nil {
		return execution.ResolutionResult{}, fmt.Errorf("lock attention attempt: %w", err)
	}
	if state != "NEEDS_ATTENTION" || (status != "AMBIGUOUS" && status != "NEEDS_ATTENTION") || revision != command.Work.ExpectedRevision || (command.Work.ExpectedCommandEpoch != nil && epoch != *command.Work.ExpectedCommandEpoch) {
		var persistedFingerprint, action, reason string
		var journalConfirmedAbsent, lateCallerExcluded bool
		var lateCallerExclusionMethod, proofReference *string
		err = tx.QueryRow(ctx, `SELECT evidence_fingerprint,evidence_summary->>'action',reason_code,journal_confirmed_absent,late_caller_excluded,late_caller_exclusion_method,proof_reference
FROM ambiguity_resolutions WHERE attempt_id=$1::uuid AND command_id=$2::uuid`, command.AttemptID, command.Work.CommandID).Scan(
			&persistedFingerprint, &action, &reason, &journalConfirmedAbsent, &lateCallerExcluded, &lateCallerExclusionMethod, &proofReference,
		)
		if err == nil && persistedFingerprint == command.EvidenceFingerprint && action == command.Action && reason == command.Reason && journalConfirmedAbsent == command.JournalConfirmedAbsent && lateCallerExcluded == command.LateCallerExcluded && nullableStringEqual(lateCallerExclusionMethod, command.LateCallerExclusionMethod) && nullableStringEqual(proofReference, command.ProofReference) {
			if err = tx.Commit(ctx); err != nil {
				return execution.ResolutionResult{}, classifyDueWorkError(err, "commit idempotent attention resolution")
			}
			return execution.ResolutionResult{TargetUpdateID: updateID}, nil
		}
		return execution.ResolutionResult{}, fmt.Errorf("%w: attention resolution fence changed", ErrLeaseConflict)
	}
	outcome, nextState, nextStatus, nextWork, resume, err := adminResolutionTransition(command)
	if err != nil {
		return execution.ResolutionResult{}, err
	}
	evidence, _ := json.Marshal(map[string]any{"action": command.Action, "reason": command.Reason, "principal": command.Principal.Subject, "issuer_class": command.Principal.IssuerClass, "proof_reference": command.ProofReference, "late_caller_exclusion_method": command.LateCallerExclusionMethod})
	if _, err = tx.Exec(ctx, `INSERT INTO ambiguity_resolutions (id,attempt_id,target_update_revision,outcome,evidence_summary,evidence_fingerprint,journal_confirmed_absent,late_caller_excluded,late_caller_exclusion_method,proof_reference,actor_id,command_id,reason_code)
VALUES (md5($1)::uuid,$2::uuid,$3,$4,$5::jsonb,$6,$7,$8,$9,$10,$11,$12::uuid,$13)`, "fleetforge.admin-resolution/v1:"+command.AttemptID+":"+command.Work.CommandID, command.AttemptID, revision, outcome, evidence, command.EvidenceFingerprint, command.JournalConfirmedAbsent, command.LateCallerExcluded, nullableExecutionString(command.LateCallerExclusionMethod), nullableExecutionString(command.ProofReference), command.Principal.Subject, command.Work.CommandID, command.Reason); err != nil {
		return execution.ResolutionResult{}, fmt.Errorf("persist administrator resolution: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE firmware_update_attempts SET status=$2,resolved_at=$3 WHERE id=$1::uuid AND status IN ('AMBIGUOUS','NEEDS_ATTENTION')`, command.AttemptID, nextStatus, command.Now.UTC()); err != nil {
		return execution.ResolutionResult{}, fmt.Errorf("advance resolved attempt: %w", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE target_updates SET state=$2,resume_step=$3,revision=revision+1,updated_at=$4 WHERE id=$1::uuid AND revision=$5 AND command_epoch=$6`, updateID, nextState, nullableExecutionString(resume), command.Now.UTC(), revision, epoch)
	if err != nil {
		return execution.ResolutionResult{}, fmt.Errorf("advance administrator resolution: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return execution.ResolutionResult{}, fmt.Errorf("%w: target update fence changed", ErrLeaseConflict)
	}
	if command.Action == "RETRY_PROVEN_ABSENT" {
		if _, err = tx.Exec(ctx, `UPDATE submission_admissions SET retry_authorizations=retry_authorizations+1 WHERE id=$1::uuid`, admissionID); err != nil {
			return execution.ResolutionResult{}, fmt.Errorf("authorize later retry: %w", err)
		}
	}
	if nextWork != "" {
		if _, err = tx.Exec(ctx, `INSERT INTO due_work (id,command_id,subject_type,subject_id,work_type,schema_version,not_before,expected_revision,expected_command_epoch)
VALUES (md5($1)::uuid,md5($2)::uuid,'TARGET_UPDATE',$3::uuid,$4,1,$5,$6,$7)`, "fleetforge.execution.work/v1:"+command.AttemptID+":"+nextWork, "fleetforge.execution.command/v1:"+command.AttemptID+":"+nextWork, updateID, nextWork, command.Now.UTC(), revision+1, epoch); err != nil {
			return execution.ResolutionResult{}, fmt.Errorf("schedule adopted observation: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return execution.ResolutionResult{}, classifyDueWorkError(err, "commit attention resolution")
	}
	return execution.ResolutionResult{TargetUpdateID: updateID}, nil
}

func validResolutionPrincipal(principal execution.VerifiedPrincipal, now time.Time) bool {
	return principal.Verified && principal.Subject != "" && !principal.AuthenticatedAt.IsZero() && !principal.AuthenticatedAt.After(now) && now.Sub(principal.AuthenticatedAt) <= 15*time.Minute && !principal.ExpiresAt.IsZero() && principal.ExpiresAt.After(now) && slices.Contains(principal.Roles, "firmware:resolve")
}

func adminResolutionTransition(command execution.AdminResolutionCommand) (outcome, state, attemptStatus, work, resume string, err error) {
	switch command.Action {
	case "ADOPT":
		return "ADOPTED", "UPDATE_ACCEPTED", "ADOPTED", "TASK_OBSERVE", "", nil
	case "TERMINAL":
		return "TERMINAL_FAILURE", "FAILED_TERMINAL", "FAILED_TERMINAL", "", "", nil
	case "RETRY_PROVEN_ABSENT":
		if !command.JournalConfirmedAbsent || !command.LateCallerExcluded || command.LateCallerExclusionMethod == "" || command.ProofReference == "" {
			return "", "", "", "", "", fmt.Errorf("retry resolution lacks durable absence proof")
		}
		return "PROVEN_ABSENT", "RETRY_WAIT", "PROVEN_ABSENT", "RETRY_PRECHECK", "SUBMIT_ADMISSION", nil
	default:
		return "", "", "", "", "", fmt.Errorf("unsupported attention resolution action")
	}
}

func validReconciliationDecision(decision execution.ReconciliationDecision) bool {
	if decision.Outcome == "" || decision.EvidenceFingerprint == "" || decision.ReasonCode == "" {
		return false
	}
	switch decision.Outcome {
	case "ADOPT":
		return !decision.ProvenAbsent && !decision.LateCallerExcluded
	case "NEEDS_ATTENTION":
		return !decision.ProvenAbsent && !decision.LateCallerExcluded
	case "PROVEN_ABSENT":
		return decision.ProvenAbsent && decision.LateCallerExcluded
	default:
		return false
	}
}

func reconciliationTransition(outcome string) (state, attemptStatus, work, resume string) {
	switch outcome {
	case "ADOPT":
		return "UPDATE_ACCEPTED", "ADOPTED", "TASK_OBSERVE", ""
	case "PROVEN_ABSENT":
		return "RETRY_WAIT", "PROVEN_ABSENT", "RETRY_PRECHECK", "SUBMIT_ADMISSION"
	default:
		return "NEEDS_ATTENTION", "NEEDS_ATTENTION", "", ""
	}
}

func resolutionOutcome(outcome string) string {
	if outcome == "ADOPT" {
		return "ADOPTED"
	}
	return outcome
}

func nullableInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableStringEqual(value *string, expected string) bool {
	if value == nil {
		return expected == ""
	}
	return *value == expected
}
