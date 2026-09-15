// mapper.go — strict row-to-domain type conversions. Unknown persisted values
// produce ErrCorruptPersistedValue and must never be silently coerced.
package postgres

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/campaign"
	"github.com/parth10-05/fleetforge/internal/domain/reason"
	"github.com/parth10-05/fleetforge/internal/domain/targetupdate"
)

// ErrCorruptPersistedValue is returned when a row loaded from the database
// contains a value that is not in the accepted Phase 02 vocabulary. Callers
// must treat this as a non-retryable integrity failure.
var ErrCorruptPersistedValue = errors.New("corrupt persisted value")

// ─── Campaign row ────────────────────────────────────────────────────────────

// CampaignRow is a persistence-only type that mirrors the database row.
// It must not be exposed outside this package.
type CampaignRow struct {
	ID                       string
	ArtifactID               string
	State                    string
	Revision                 int64
	CommandEpoch             int64
	SuspendedPhase           *string
	PolicyVersionID          string
	FrozenPolicyHash         *string
	FrozenTargetSnapshotHash *string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// CampaignRowState parses the raw state string from a CampaignRow using the
// Phase 02 campaign.ParseState parser. Unknown values fail closed.
func CampaignRowState(raw string) (campaign.State, error) {
	state, err := campaign.ParseState(raw)
	if err != nil {
		return "", fmt.Errorf("%w: campaign state %q", ErrCorruptPersistedValue, raw)
	}
	return state, nil
}

// CampaignRowSuspendedPhase parses the nullable suspended_phase column.
// A nil pointer returns ("", nil). An unknown value fails closed.
func CampaignRowSuspendedPhase(raw *string) (campaign.State, error) {
	if raw == nil {
		return "", nil
	}
	state, err := campaign.ParseState(*raw)
	if err != nil {
		return "", fmt.Errorf("%w: campaign suspended_phase %q", ErrCorruptPersistedValue, *raw)
	}
	// Only the three suspendable states are legal as suspended_phase.
	switch state {
	case campaign.StateRunningCanary, campaign.StateEvaluatingCanary, campaign.StateRunningWaves:
		return state, nil
	}
	return "", fmt.Errorf("%w: campaign suspended_phase %q is not a suspendable state", ErrCorruptPersistedValue, *raw)
}

// ─── TargetUpdate row ────────────────────────────────────────────────────────

// TargetUpdateRow is a persistence-only type that mirrors the database row.
type TargetUpdateRow struct {
	ID                      string
	CampaignTargetID        string
	State                   string
	Revision                int64
	CommandEpoch            int64
	ResumeStep              *string
	ExpectedFirmwareVersion string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// TargetUpdateRowState parses the raw state using the Phase 02 parser.
func TargetUpdateRowState(raw string) (targetupdate.State, error) {
	state, err := targetupdate.ParseState(raw)
	if err != nil {
		return "", fmt.Errorf("%w: target_update state %q", ErrCorruptPersistedValue, raw)
	}
	return state, nil
}

// TargetUpdateRowResumeStep parses the nullable resume_step column.
func TargetUpdateRowResumeStep(raw *string) (targetupdate.ResumeStep, error) {
	if raw == nil {
		return "", nil
	}
	step, err := targetupdate.ParseResumeStep(*raw)
	if err != nil {
		return "", fmt.Errorf("%w: target_update resume_step %q", ErrCorruptPersistedValue, *raw)
	}
	return step, nil
}

// ─── AuditEvent row ──────────────────────────────────────────────────────────

// AuditEventRow is a persistence-only type for writing audit events.
type AuditEventRow struct {
	AggregateType     string
	AggregateID       string
	AggregateRevision int64
	EventSequence     int64
	ActorID           string
	CommandID         *string
	Action            string
	ReasonCode        string
	BeforeHash        *string
	AfterHash         *string
}

// AuditEventRowReasonCode parses the reason_code column using the Phase 02 parser.
func AuditEventRowReasonCode(raw string) (reason.Code, error) {
	code, err := reason.ParseCode(raw)
	if err != nil {
		return "", fmt.Errorf("%w: audit_event reason_code %q", ErrCorruptPersistedValue, raw)
	}
	return code, nil
}

// ─── OutboxEvent row ──────────────────────────────────────────────────────────

// OutboxEventRow is a persistence-only type for writing outbox event envelopes.
type OutboxEventRow struct {
	EventID           string
	AggregateType     string
	AggregateID       string
	AggregateRevision int64
	EventType         string
	SchemaVersion     int
	Payload           json.RawMessage
}

// ─── DueWork row ─────────────────────────────────────────────────────────────

// DueWorkRow is a persistence-only type for scheduling due-work intents.
type DueWorkRow struct {
	ID                   string
	CommandID            string
	SubjectType          string
	SubjectID            string
	WorkType             string
	SchemaVersion        int
	NotBefore            time.Time
	ExpectedRevision     int64
	ExpectedCommandEpoch *int64
}

// DueWorkSubjectType is the set of accepted due_work.subject_type values.
// The check constraint in the schema defines the canonical list; this constant
// set allows the mapper to fail closed on load without re-checking the database.
type DueWorkSubjectType string

const (
	DueWorkSubjectTargetUpdate DueWorkSubjectType = "TARGET_UPDATE"
	DueWorkSubjectCampaign     DueWorkSubjectType = "CAMPAIGN"
	DueWorkSubjectArtifact     DueWorkSubjectType = "ARTIFACT"
	DueWorkSubjectValidation   DueWorkSubjectType = "VALIDATION"
)

var allDueWorkSubjectTypes = []DueWorkSubjectType{
	DueWorkSubjectTargetUpdate,
	DueWorkSubjectCampaign,
	DueWorkSubjectArtifact,
	DueWorkSubjectValidation,
}

// ParseDueWorkSubjectType accepts only a registered subject type. Unknown
// values from the database indicate corruption and fail closed.
func ParseDueWorkSubjectType(raw string) (DueWorkSubjectType, error) {
	candidate := DueWorkSubjectType(raw)
	for _, known := range allDueWorkSubjectTypes {
		if candidate == known {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: due_work subject_type %q", ErrCorruptPersistedValue, raw)
}

// ─── IdempotencyRecord row ────────────────────────────────────────────────────

// IdempotencyState is the accepted vocabulary for idempotency_records.state.
type IdempotencyState string

const (
	IdempotencyStateInProgress       IdempotencyState = "IN_PROGRESS"
	IdempotencyStateCompleted        IdempotencyState = "COMPLETED"
	IdempotencyStateRecoveryRequired IdempotencyState = "RECOVERY_REQUIRED"
)

var allIdempotencyStates = []IdempotencyState{
	IdempotencyStateInProgress,
	IdempotencyStateCompleted,
	IdempotencyStateRecoveryRequired,
}

// ParseIdempotencyState accepts only a registered state value.
func ParseIdempotencyState(raw string) (IdempotencyState, error) {
	candidate := IdempotencyState(raw)
	for _, known := range allIdempotencyStates {
		if candidate == known {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: idempotency_records state %q", ErrCorruptPersistedValue, raw)
}

// ─── DecisionInput row ────────────────────────────────────────────────────────

// DecisionInputAggregateType is the accepted vocabulary for decision_inputs.aggregate_type.
type DecisionInputAggregateType string

const (
	DecisionInputAggregateCampaign     DecisionInputAggregateType = "CAMPAIGN"
	DecisionInputAggregateTargetUpdate DecisionInputAggregateType = "TARGET_UPDATE"
)

var allDecisionInputAggregateTypes = []DecisionInputAggregateType{
	DecisionInputAggregateCampaign,
	DecisionInputAggregateTargetUpdate,
}

// ParseDecisionInputAggregateType accepts only a registered aggregate type.
func ParseDecisionInputAggregateType(raw string) (DecisionInputAggregateType, error) {
	candidate := DecisionInputAggregateType(raw)
	for _, known := range allDecisionInputAggregateTypes {
		if candidate == known {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: decision_inputs aggregate_type %q", ErrCorruptPersistedValue, raw)
}
