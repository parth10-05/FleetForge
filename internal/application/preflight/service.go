// Package preflight coordinates bounded, read-only candidate evaluation.
package preflight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/domain/inventory"
	domainpreflight "github.com/parth10-05/fleetforge/internal/domain/preflight"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

var ErrBlocked = errors.New("preflight is blocked")

type Target struct {
	ID       value.TargetID
	Revision value.Revision
	Labels   value.FailureDomainLabels
	Family   value.LabelValue
}
type Artifact struct {
	ID     value.ArtifactID
	State  artifact.State
	Family value.LabelValue
	Object artifact.ObjectIdentity
}
type TargetSelector interface {
	Select(context.Context, inventory.Selector, int) ([]Target, error)
}
type ArtifactReader interface {
	Artifact(context.Context, value.ArtifactID) (Artifact, error)
}
type CandidateWriter interface {
	Persist(context.Context, string, domainpreflight.Snapshot, []byte) error
}
type Request struct {
	CandidateID string
	ArtifactID  value.ArtifactID
	Selector    []byte
	Maximum     int
}
type Result struct {
	Snapshot domainpreflight.Snapshot
	Report   []byte
}
type Service struct {
	targets    TargetSelector
	artifacts  ArtifactReader
	candidates CandidateWriter
}

func New(targets TargetSelector, artifacts ArtifactReader, candidates CandidateWriter) *Service {
	return &Service{targets: targets, artifacts: artifacts, candidates: candidates}
}

// Evaluate computes and persists one immutable passed snapshot. The service
// holds no database lock and performs no network or object-store I/O itself.
// EvaluateSnapshot computes one immutable passed snapshot without writing it.
// Callers that need atomic composition with other durable effects use this
// method, keeping evaluation outside their final transaction.
func (service *Service) EvaluateSnapshot(ctx context.Context, request Request) (Result, error) {
	if service == nil || service.targets == nil || service.artifacts == nil || request.CandidateID == "" || request.ArtifactID.IsZero() || request.Maximum <= 0 || request.Maximum > 100 {
		return Result{}, fmt.Errorf("invalid preflight request")
	}
	selector, err := inventory.ParseSelector(request.Selector)
	if err != nil {
		return Result{}, fmt.Errorf("parse preflight selector: %w", err)
	}
	artifactRecord, err := service.artifacts.Artifact(ctx, request.ArtifactID)
	if err != nil {
		return Result{}, fmt.Errorf("load preflight artifact: %w", err)
	}
	if artifactRecord.ID != request.ArtifactID || artifactRecord.State != artifact.StateVerified {
		return Result{}, fmt.Errorf("%w: artifact is not verified", ErrBlocked)
	}
	targets, err := service.targets.Select(ctx, selector, request.Maximum)
	if err != nil {
		return Result{}, fmt.Errorf("select preflight targets: %w", err)
	}
	if len(targets) == 0 {
		return Result{}, fmt.Errorf("%w: selector matched no targets", ErrBlocked)
	}
	frozen := make([]domainpreflight.Target, 0, len(targets))
	for _, target := range targets {
		decision, err := artifact.EvaluateCompatibility(artifact.CompatibilityPolicyV1, artifactRecord.State, artifactRecord.Family, target.Family)
		if err != nil {
			return Result{}, fmt.Errorf("evaluate compatibility: %w", err)
		}
		if !decision.Compatible() {
			return Result{}, fmt.Errorf("%w: target %s: %s", ErrBlocked, target.ID.String(), decision.Reason())
		}
		frozen = append(frozen, domainpreflight.Target{ID: target.ID, Revision: target.Revision, Labels: target.Labels, Family: target.Family, Compatibility: decision})
	}
	snapshot, err := domainpreflight.NewSnapshot(selector, artifactRecord.ID, artifactRecord.Object, frozen)
	if err != nil {
		return Result{}, fmt.Errorf("build preflight snapshot: %w", err)
	}
	report, err := json.Marshal(struct {
		State       string `json:"state"`
		TargetCount int    `json:"target_count"`
	}{State: "passed", TargetCount: len(frozen)})
	if err != nil {
		return Result{}, fmt.Errorf("encode preflight report: %w", err)
	}
	return Result{Snapshot: snapshot, Report: report}, nil
}

// Evaluate preserves the existing persisted-candidate contract.
func (service *Service) Evaluate(ctx context.Context, request Request) (Result, error) {
	if service == nil || service.candidates == nil {
		return Result{}, fmt.Errorf("invalid preflight request")
	}
	result, err := service.EvaluateSnapshot(ctx, request)
	if err != nil {
		return Result{}, err
	}
	if err := service.candidates.Persist(ctx, request.CandidateID, result.Snapshot, result.Report); err != nil {
		return Result{}, fmt.Errorf("persist preflight candidate: %w", err)
	}
	return result, nil
}
