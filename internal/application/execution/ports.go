// Package execution owns provider-neutral firmware execution contracts.
package execution

import (
	"context"
	"io"
	"time"
)

type WorkFence struct {
	DueWorkID, CommandID, LeaseToken     string
	DeliveryGeneration, ExpectedRevision int64
	ExpectedCommandEpoch                 *int64
}
type ObjectIdentity struct {
	Provider, Container, ObjectKey, ObjectVersion, SHA256 string
	Size                                                  int64
}
type PrecheckSnapshot struct {
	TargetID, CampaignID, TargetUpdateID, ArtifactID, ExpectedVersion string
	CommandEpoch, Revision                                            int64
}
type PrecheckRequest struct{ TargetID, CredentialReference, ExpectedVersion string }
type PrecheckEvidence struct {
	CurrentVersion               string
	Capable, Healthy, Compatible bool
	EvidenceFingerprint          string
	ObservedAt                   time.Time
}
type AdmissionCommand struct {
	Work                                  WorkFence
	Snapshot                              PrecheckSnapshot
	Artifact                              ObjectIdentity
	AttemptID, StepAttemptID, AdmissionID string
}
type AdmissionResult struct{ AttemptID, SubmitWorkID, SubmitCommandID string }
type AttemptSnapshot struct {
	AttemptID, ParentAttemptID, AdmissionID, TargetID, TargetUpdateID, CampaignID string
	Artifact                                                                      ObjectIdentity
	ExpectedVersion, CredentialReference, RequestFingerprint                      string
	CommandEpoch, AdmittedRevision                                                int64
}
type SubmitRequest struct {
	Attempt          AttemptSnapshot
	UpdateActionPath string
}
type SubmitClass string

const (
	SubmitAccepted              SubmitClass = "ACCEPTED"
	SubmitExplicitRejection     SubmitClass = "EXPLICIT_REJECTION"
	SubmitProvenPrewriteFailure SubmitClass = "PROVEN_PREWRITE_FAILURE"
	SubmitPossiblyAccepted      SubmitClass = "POSSIBLY_ACCEPTED"
	SubmitUnsafeResponse        SubmitClass = "UNSAFE_RESPONSE"
)

type SubmitResult struct {
	Class                                    SubmitClass
	RemoteTaskReference, ResponseFingerprint string
	FinishedAt                               time.Time
}
type ObservationKind string

const (
	ObservationTask      ObservationKind = "TASK"
	ObservationInventory ObservationKind = "INVENTORY"
	ObservationHealth    ObservationKind = "HEALTH"
)

type Observation struct {
	Kind                                                                                   ObservationKind
	TargetID, TargetUpdateID, AttemptID, ResourcePath, RemoteSequence, EvidenceFingerprint string
	Data                                                                                   []byte
	ObservedAt, ReceivedAt                                                                 time.Time
	Fresh, ContradictsExpectedState                                                        bool
}
type TaskObservationRequest struct{ Attempt AttemptSnapshot }
type InventoryObservationRequest struct{ Attempt AttemptSnapshot }
type HealthObservationRequest struct{ Attempt AttemptSnapshot }
type ObservationDecision struct {
	Work         WorkFence
	Observation  Observation
	NextWorkType string
}
type ReconciliationSnapshot struct{ Attempt AttemptSnapshot }
type ReconciliationDecision struct {
	Work                                                          WorkFence
	ProvenAbsent, LateCallerExcluded                              bool
	EvidenceFingerprint, JournalRunID, CheckpointHash, ReasonCode string
	JournalSequenceStart, JournalSequenceEnd                      int64
}
type VerifiedPrincipal struct {
	Subject, IssuerClass       string
	Roles                      []string
	AuthenticatedAt, ExpiresAt time.Time
	Verified                   bool
}
type AdminResolutionCommand struct {
	Principal                                      VerifiedPrincipal
	Work                                           WorkFence
	AttemptID, Action, Reason, EvidenceFingerprint string
	Now                                            time.Time
}
type ResolutionResult struct{ TargetUpdateID string }

type ExecutionRepository interface {
	LoadPrecheck(context.Context, WorkFence) (PrecheckSnapshot, error)
	AdmitSubmission(context.Context, AdmissionCommand) (AdmissionResult, error)
	LoadAttempt(context.Context, string) (AttemptSnapshot, error)
	CommitSubmitResult(context.Context, WorkFence, SubmitResult) error
	CommitObservation(context.Context, WorkFence, ObservationDecision) error
	LoadReconciliation(context.Context, WorkFence) (ReconciliationSnapshot, error)
	CommitReconciliation(context.Context, WorkFence, ReconciliationDecision) error
	ResolveAttention(context.Context, AdminResolutionCommand) (ResolutionResult, error)
}
type RedfishExecutionPort interface {
	Precheck(context.Context, PrecheckRequest) (PrecheckEvidence, error)
	Submit(context.Context, SubmitRequest, io.Reader) SubmitResult
	ObserveTask(context.Context, TaskObservationRequest) (Observation, error)
	ObserveInventory(context.Context, InventoryObservationRequest) (Observation, error)
	ObserveHealth(context.Context, HealthObservationRequest) (Observation, error)
}
type ExactArtifactReader interface {
	OpenExact(context.Context, ObjectIdentity) (io.ReadCloser, error)
}
type IndependentAcceptanceReader interface {
	ReadAcceptance(context.Context, string, string, time.Time, time.Time) (AcceptanceJournal, error)
}
type AcceptanceJournal struct {
	RunID, CheckpointHash string
	Entries               []AcceptanceEntry
}
type AcceptanceEntry struct {
	AttemptID, RequestFingerprint string
	Received, Accepted            bool
	Sequence                      int64
}
type TokenVerifier interface {
	VerifyAccessToken(context.Context, string) (VerifiedPrincipal, error)
}
