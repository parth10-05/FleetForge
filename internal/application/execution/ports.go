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

// CapacityLimit and AdmissionEnvelope carry the already-authorized,
// immutable policy decision into the short persistence transaction after
// precheck I/O completes. The execution worker cannot derive or relax these
// values from target responses.
type CapacityLimit struct {
	Level, Key string
	Limit      int64
}
type AdmissionEnvelope struct {
	CampaignTargetID, ExpectedTargetState, NextTargetState  string
	ResumeStep                                              *string
	Step                                                    string
	Global, Site, Rack                                      CapacityLimit
	ReservationIDs                                          [3]string
	ActorID, CorrelationID, CausationID, Action, ReasonCode string
	AuditEventSequence                                      int64
	OutboxEventID, OutboxEventType                          string
	OutboxSchemaVersion                                     int
	OutboxPayload                                           []byte
	DueWorkID, DueWorkWorkType                              string
	DueWorkSchemaVersion                                    int
	DueWorkNotBefore                                        time.Time
	DecisionInputID                                         string
	DecisionSequence                                        int64
	DecisionLogicalTime                                     time.Time
	ReducerVersion, DecisionPolicyVersion                   string
	NormalizedFacts                                         []byte
	DecisionResult                                          string
}

// CompletionEnvelope is the immutable decision/audit envelope for a
// precheck that proves an update is already active and healthy. Unlike an
// admission it deliberately contains no capacity reservation or follow-up
// submission work: completing this route must never create a target write.
type CompletionEnvelope struct {
	CampaignTargetID, ExpectedTargetState, NextTargetState  string
	ActorID, CorrelationID, CausationID, Action, ReasonCode string
	AuditEventSequence                                      int64
	OutboxEventID, OutboxEventType                          string
	OutboxSchemaVersion                                     int
	OutboxPayload                                           []byte
	DecisionInputID                                         string
	DecisionSequence                                        int64
	DecisionLogicalTime                                     time.Time
	ReducerVersion, DecisionPolicyVersion                   string
	NormalizedFacts                                         []byte
	DecisionResult                                          string
}
type AdmissionCommand struct {
	Work                                                      WorkFence
	Snapshot                                                  PrecheckSnapshot
	Artifact                                                  ObjectIdentity
	AttemptID, StepAttemptID, AdmissionID, RequestFingerprint string
	// RetryParentAttemptID is set only for a proof-backed retry. It keeps the
	// admission worker on the safe-read path while the persistence boundary
	// creates a new child attempt under the existing semantic admission.
	RetryParentAttemptID   string
	PrecheckEvidence       PrecheckEvidence
	Envelope               AdmissionEnvelope
	AlreadyCurrentEnvelope CompletionEnvelope
}
type AdmissionResult struct {
	AttemptID, SubmitWorkID, SubmitCommandID string
	AlreadyCurrent                           bool
}
type AlreadyCurrentCommand struct {
	Work     WorkFence
	Snapshot PrecheckSnapshot
	Evidence PrecheckEvidence
	Envelope CompletionEnvelope
}
type AttemptSnapshot struct {
	AttemptID, ParentAttemptID, AdmissionID, TargetID, TargetUpdateID, CampaignID string
	Artifact                                                                      ObjectIdentity
	ExpectedVersion, CredentialReference, RequestFingerprint, RemoteTaskReference string
	CommandEpoch, AdmittedRevision                                                int64
	ObservationFreshnessSeconds                                                   uint64
	StartedAt                                                                     time.Time
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
	ObservationReboot    ObservationKind = "REBOOT"
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
type RebootObservationRequest struct{ Attempt AttemptSnapshot }
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
	Outcome                                                       string
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
	JournalConfirmedAbsent, LateCallerExcluded     bool
	LateCallerExclusionMethod, ProofReference      string
	Now                                            time.Time
}
type ResolutionResult struct{ TargetUpdateID string }

type ExecutionRepository interface {
	LoadPrecheck(context.Context, WorkFence) (PrecheckSnapshot, error)
	AdmitSubmission(context.Context, AdmissionCommand) (AdmissionResult, error)
	AdmitRetrySubmission(context.Context, AdmissionCommand) (AdmissionResult, error)
	CompleteAlreadyCurrent(context.Context, AlreadyCurrentCommand) error
	LoadAttempt(context.Context, WorkFence) (AttemptSnapshot, error)
	CommitSubmitResult(context.Context, WorkFence, SubmitResult) error
	CommitObservation(context.Context, WorkFence, ObservationDecision) error
	LoadReconciliation(context.Context, WorkFence) (ReconciliationSnapshot, error)
	CommitReconciliation(context.Context, WorkFence, ReconciliationDecision) error
	ResolveAttention(context.Context, AdminResolutionCommand) (ResolutionResult, error)
}

// SubmitRepository is intentionally narrower than ExecutionRepository. The
// one-shot side-effect coordinator must have no access to retry, observation,
// or administrative-resolution operations.
type SubmitRepository interface {
	LoadAttempt(context.Context, WorkFence) (AttemptSnapshot, error)
	CommitSubmitResult(context.Context, WorkFence, SubmitResult) error
}

type SubmitTarget interface {
	Submit(context.Context, SubmitRequest, io.Reader) SubmitResult
}
type RedfishExecutionPort interface {
	Precheck(context.Context, PrecheckRequest) (PrecheckEvidence, error)
	Submit(context.Context, SubmitRequest, io.Reader) SubmitResult
	ObserveTask(context.Context, TaskObservationRequest) (Observation, error)
	ObserveReboot(context.Context, RebootObservationRequest) (Observation, error)
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
