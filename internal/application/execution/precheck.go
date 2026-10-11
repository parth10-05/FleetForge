package execution

import (
	"context"
	"fmt"
)

// PrecheckRepository is deliberately limited to the read required before
// target I/O and the subsequent durable admission transaction.
type PrecheckRepository interface {
	LoadPrecheck(context.Context, WorkFence) (PrecheckSnapshot, error)
	AdmitSubmission(context.Context, AdmissionCommand) (AdmissionResult, error)
	CompleteAlreadyCurrent(context.Context, AlreadyCurrentCommand) error
}

type PrecheckTarget interface {
	Precheck(context.Context, PrecheckRequest) (PrecheckEvidence, error)
}

// PrecheckAndAdmit performs bounded target I/O between two durable phases. It
// never holds a database transaction while calling the target, and it never
// converts a stale/ineligible response into a submission. An already-current
// target is instead completed through its own evidence-backed write path.
func PrecheckAndAdmit(ctx context.Context, repository PrecheckRepository, target PrecheckTarget, work WorkFence, command AdmissionCommand, credentialReference string) (AdmissionResult, PrecheckEvidence, error) {
	if repository == nil || target == nil || !validWorkFence(work) || credentialReference == "" {
		return AdmissionResult{}, PrecheckEvidence{}, fmt.Errorf("invalid precheck request")
	}
	snapshot, err := repository.LoadPrecheck(ctx, work)
	if err != nil {
		return AdmissionResult{}, PrecheckEvidence{}, fmt.Errorf("load fenced precheck: %w", err)
	}
	if snapshot.TargetID == "" || snapshot.CampaignID == "" || snapshot.TargetUpdateID == "" || snapshot.ArtifactID == "" || snapshot.ExpectedVersion == "" || snapshot.Revision != work.ExpectedRevision || snapshot.CommandEpoch < 0 {
		return AdmissionResult{}, PrecheckEvidence{}, fmt.Errorf("invalid fenced precheck snapshot")
	}
	evidence, err := target.Precheck(ctx, PrecheckRequest{TargetID: snapshot.TargetID, CredentialReference: credentialReference, ExpectedVersion: snapshot.ExpectedVersion})
	if err != nil {
		return AdmissionResult{}, PrecheckEvidence{}, fmt.Errorf("collect target precheck: %w", err)
	}
	if evidence.ObservedAt.IsZero() || evidence.EvidenceFingerprint == "" {
		return AdmissionResult{}, PrecheckEvidence{}, fmt.Errorf("target returned incomplete precheck evidence")
	}
	if evidence.CurrentVersion == snapshot.ExpectedVersion {
		if !evidence.Healthy || !validCompletionEnvelope(command.AlreadyCurrentEnvelope) {
			return AdmissionResult{}, evidence, fmt.Errorf("already-current target lacks configured health proof or completion envelope")
		}
		completion := AlreadyCurrentCommand{Work: work, Snapshot: snapshot, Evidence: evidence, Envelope: command.AlreadyCurrentEnvelope}
		if err := repository.CompleteAlreadyCurrent(ctx, completion); err != nil {
			return AdmissionResult{}, evidence, fmt.Errorf("persist already-current completion: %w", err)
		}
		return AdmissionResult{AlreadyCurrent: true}, evidence, nil
	}
	if !validAdmissionCommand(command) {
		return AdmissionResult{}, evidence, fmt.Errorf("invalid precheck admission request")
	}
	if !evidence.Capable || !evidence.Healthy || !evidence.Compatible {
		return AdmissionResult{}, evidence, fmt.Errorf("target is ineligible for firmware admission")
	}
	command.Work, command.Snapshot, command.PrecheckEvidence = work, snapshot, evidence
	result, err := repository.AdmitSubmission(ctx, command)
	if err != nil {
		return AdmissionResult{}, evidence, fmt.Errorf("persist firmware admission: %w", err)
	}
	return result, evidence, nil
}

func validWorkFence(work WorkFence) bool {
	return work.DueWorkID != "" && work.CommandID != "" && work.LeaseToken != "" && work.ExpectedRevision >= 0 && work.DeliveryGeneration >= 0
}

func validAdmissionCommand(command AdmissionCommand) bool {
	return command.AttemptID != "" && command.StepAttemptID != "" && command.AdmissionID != "" && command.RequestFingerprint != "" && command.RequestFingerprint == command.Artifact.SHA256 && command.Artifact.Provider != "" && command.Artifact.Container != "" && command.Artifact.ObjectKey != "" && command.Artifact.ObjectVersion != "" && command.Artifact.SHA256 != "" && command.Artifact.Size > 0 && validAdmissionEnvelope(command.Envelope)
}

func validAdmissionEnvelope(envelope AdmissionEnvelope) bool {
	if envelope.CampaignTargetID == "" || envelope.ExpectedTargetState == "" || envelope.NextTargetState == "" || envelope.Step == "" || envelope.ActorID == "" || envelope.CorrelationID == "" || envelope.CausationID == "" || envelope.Action == "" || envelope.ReasonCode == "" || envelope.OutboxEventID == "" || envelope.OutboxEventType == "" || envelope.OutboxSchemaVersion <= 0 || len(envelope.OutboxPayload) == 0 || envelope.DueWorkID == "" || envelope.DueWorkWorkType == "" || envelope.DueWorkSchemaVersion <= 0 || envelope.DueWorkNotBefore.IsZero() || envelope.DecisionInputID == "" || envelope.DecisionLogicalTime.IsZero() || envelope.ReducerVersion == "" || envelope.DecisionPolicyVersion == "" || len(envelope.NormalizedFacts) == 0 || envelope.DecisionResult == "" {
		return false
	}
	for _, limit := range []CapacityLimit{envelope.Global, envelope.Site, envelope.Rack} {
		if limit.Level == "" || limit.Key == "" || limit.Limit <= 0 {
			return false
		}
	}
	for _, id := range envelope.ReservationIDs {
		if id == "" {
			return false
		}
	}
	return true
}

func validCompletionEnvelope(envelope CompletionEnvelope) bool {
	return envelope.CampaignTargetID != "" && envelope.ExpectedTargetState == "PRECHECK" && envelope.NextTargetState == "SUCCEEDED" && envelope.ActorID != "" && envelope.CorrelationID != "" && envelope.CausationID != "" && envelope.Action != "" && envelope.ReasonCode != "" && envelope.OutboxEventID != "" && envelope.OutboxEventType != "" && envelope.OutboxSchemaVersion > 0 && len(envelope.OutboxPayload) > 0 && envelope.DecisionInputID != "" && !envelope.DecisionLogicalTime.IsZero() && envelope.ReducerVersion != "" && envelope.DecisionPolicyVersion != "" && len(envelope.NormalizedFacts) > 0 && envelope.DecisionResult == "SUCCEEDED"
}
