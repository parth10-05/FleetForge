package campaignvalidation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parth10-05/fleetforge/internal/application/preflight"
	"github.com/parth10-05/fleetforge/internal/domain/campaignplan"
	domainpreflight "github.com/parth10-05/fleetforge/internal/domain/preflight"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

// ValidationInput is immutable draft data loaded before validation begins.
type ValidationInput struct {
	ArtifactID, PolicyVersionID, Seed, Algorithm string
	Selector                                     []byte
	Policy                                       campaignplan.Policy
	Maximum                                      int
}

type ValidationInputLoader interface {
	LoadCampaignValidationInput(context.Context, Work) (ValidationInput, error)
}

type SnapshotEvaluator interface {
	EvaluateSnapshot(context.Context, preflight.Request) (preflight.Result, error)
}

type PlanMember struct {
	TargetID, CampaignTargetID, TargetUpdateID, EnrollmentID string
}

// FinalizationRequest contains only facts prepared before the final database
// transaction. All IDs are deterministic descendants of Work.CommandID.
type FinalizationRequest struct {
	Work                                   Work
	CandidateID, CanaryWaveID, LaterWaveID string
	DecisionInputID, OutboxEventID         string
	Snapshot                               domainpreflight.Snapshot
	Report                                 []byte
	Plan                                   campaignplan.Plan
	PolicyVersionID                        string
	Members                                []PlanMember
	Now                                    time.Time
}

type RejectionRequest struct {
	Work                           Work
	ReasonCode                     string
	DecisionInputID, OutboxEventID string
	Now                            time.Time
}

type Finalizer interface {
	FinalizeCampaignValidation(context.Context, FinalizationRequest) error
	RejectCampaignValidation(context.Context, RejectionRequest) error
}

// ValidationProcessor evaluates and plans outside a database transaction. The finalizer
// is the sole owner of campaign state, evidence, and due-work completion.
type ValidationProcessor struct {
	inputs    ValidationInputLoader
	evaluator SnapshotEvaluator
	finalizer Finalizer
	now       func() time.Time
}

func NewValidationProcessor(inputs ValidationInputLoader, evaluator SnapshotEvaluator, finalizer Finalizer, now func() time.Time) *ValidationProcessor {
	if now == nil {
		now = time.Now
	}
	return &ValidationProcessor{inputs: inputs, evaluator: evaluator, finalizer: finalizer, now: now}
}

func (processor *ValidationProcessor) ValidateCampaign(ctx context.Context, work Work) error {
	if processor == nil || processor.inputs == nil || processor.evaluator == nil || processor.finalizer == nil || work.ID == "" || work.CommandID == "" || work.CampaignID == "" || work.LeaseToken == "" || work.ExpectedRevision < 0 {
		return fmt.Errorf("invalid campaign validation processor request")
	}
	input, err := processor.inputs.LoadCampaignValidationInput(ctx, work)
	if err != nil {
		var conflict ValidationConflict
		if errors.As(err, &conflict) {
			return conflict
		}
		return ValidationRetryable{Err: fmt.Errorf("load campaign validation input: %w", err)}
	}
	artifactID, err := value.ParseArtifactID(input.ArtifactID)
	if err != nil || input.PolicyVersionID == "" || input.Maximum <= 0 {
		return processor.reject(ctx, work, "campaign.validation.invalid_draft")
	}
	candidateID, err := derivedID(work.CommandID, "candidate", work.CampaignID)
	if err != nil {
		return fmt.Errorf("derive candidate identity: %w", err)
	}
	result, err := processor.evaluator.EvaluateSnapshot(ctx, preflight.Request{CandidateID: candidateID, ArtifactID: artifactID, Selector: input.Selector, Maximum: input.Maximum})
	if err != nil {
		if errors.Is(err, preflight.ErrBlocked) {
			return processor.reject(ctx, work, "campaign.validation.blocked")
		}
		return ValidationRetryable{Err: fmt.Errorf("evaluate campaign validation snapshot: %w", err)}
	}
	request, err := prepareFinalization(work, candidateID, input, result, processor.now().UTC())
	if err != nil {
		return processor.reject(ctx, work, "campaign.validation.invalid_plan")
	}
	if err := processor.finalizer.FinalizeCampaignValidation(ctx, request); err != nil {
		var rejected ValidationRejected
		if errors.As(err, &rejected) {
			return processor.reject(ctx, work, "campaign.validation.blocked")
		}
		return err
	}
	return nil
}

func (processor *ValidationProcessor) reject(ctx context.Context, work Work, reasonCode string) error {
	decisionID, err := derivedID(work.CommandID, "rejection-decision", reasonCode)
	if err != nil {
		return err
	}
	outboxID, err := derivedID(work.CommandID, "rejection-outbox", reasonCode)
	if err != nil {
		return err
	}
	if err := processor.finalizer.RejectCampaignValidation(ctx, RejectionRequest{Work: work, ReasonCode: reasonCode, DecisionInputID: decisionID, OutboxEventID: outboxID, Now: processor.now().UTC()}); err != nil {
		return err
	}
	return nil
}

func prepareFinalization(work Work, candidateID string, input ValidationInput, result preflight.Result, now time.Time) (FinalizationRequest, error) {
	candidates := make([]campaignplan.Candidate, 0, len(result.Snapshot.Targets()))
	for _, target := range result.Snapshot.Targets() {
		candidates = append(candidates, campaignplan.Candidate{TargetID: target.ID, Revision: target.Revision, Site: target.Labels.Site().String(), Rack: target.Labels.Rack().String(), Family: target.Family.String()})
	}
	plan, err := campaignplan.Build(campaignplan.Input{Policy: input.Policy, Seed: input.Seed, Algorithm: input.Algorithm, Candidates: candidates})
	if err != nil {
		return FinalizationRequest{}, err
	}
	canaryWaveID, err := derivedID(work.CommandID, "wave", "canary")
	if err != nil {
		return FinalizationRequest{}, err
	}
	laterWaveID, err := derivedID(work.CommandID, "wave", "later")
	if err != nil {
		return FinalizationRequest{}, err
	}
	decisionID, err := derivedID(work.CommandID, "ready-decision", work.CampaignID)
	if err != nil {
		return FinalizationRequest{}, err
	}
	outboxID, err := derivedID(work.CommandID, "ready-outbox", work.CampaignID)
	if err != nil {
		return FinalizationRequest{}, err
	}
	members := make([]PlanMember, 0, len(plan.Assignments))
	for _, assignment := range plan.Assignments {
		campaignTargetID, err := derivedID(work.CommandID, "campaign-target", assignment.TargetID)
		if err != nil {
			return FinalizationRequest{}, err
		}
		targetUpdateID, err := derivedID(work.CommandID, "target-update", assignment.TargetID)
		if err != nil {
			return FinalizationRequest{}, err
		}
		enrollmentID, err := derivedID(work.CommandID, "enrollment", assignment.TargetID)
		if err != nil {
			return FinalizationRequest{}, err
		}
		members = append(members, PlanMember{TargetID: assignment.TargetID, CampaignTargetID: campaignTargetID, TargetUpdateID: targetUpdateID, EnrollmentID: enrollmentID})
	}
	return FinalizationRequest{Work: work, CandidateID: candidateID, CanaryWaveID: canaryWaveID, LaterWaveID: laterWaveID, DecisionInputID: decisionID, OutboxEventID: outboxID, Snapshot: result.Snapshot, Report: append([]byte(nil), result.Report...), Plan: plan, PolicyVersionID: input.PolicyVersionID, Members: members, Now: now}, nil
}

func derivedID(commandID, label, identity string) (string, error) {
	command, err := uuid.Parse(commandID)
	if err != nil || label == "" || identity == "" {
		return "", fmt.Errorf("invalid deterministic identity")
	}
	return uuid.NewSHA1(command, []byte("fleetforge.campaign-validation/v1:"+label+":"+identity)).String(), nil
}
