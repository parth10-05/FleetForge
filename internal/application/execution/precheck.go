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
}

type PrecheckTarget interface {
	Precheck(context.Context, PrecheckRequest) (PrecheckEvidence, error)
}

// PrecheckAndAdmit performs bounded target I/O between two durable phases. It
// never holds a database transaction while calling the target, and it never
// converts a stale/ineligible/already-current response into a submission.
func PrecheckAndAdmit(ctx context.Context, repository PrecheckRepository, target PrecheckTarget, work WorkFence, command AdmissionCommand, credentialReference string) (AdmissionResult, PrecheckEvidence, error) {
	if repository == nil || target == nil || work.DueWorkID == "" || work.CommandID == "" || work.LeaseToken == "" || work.ExpectedRevision < 0 || work.DeliveryGeneration < 0 || credentialReference == "" || command.AttemptID == "" || command.StepAttemptID == "" || command.AdmissionID == "" || command.RequestFingerprint == "" || command.Artifact.Provider == "" || command.Artifact.Container == "" || command.Artifact.ObjectKey == "" || command.Artifact.ObjectVersion == "" || command.Artifact.SHA256 == "" || command.Artifact.Size <= 0 || !validAdmissionEnvelope(command.Envelope) {
		return AdmissionResult{}, PrecheckEvidence{}, fmt.Errorf("invalid precheck admission request")
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
		return AdmissionResult{}, evidence, fmt.Errorf("target is already at expected version")
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
