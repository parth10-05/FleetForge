package execution

import (
	"context"
	"encoding/json"
	"fmt"
)

const retryObservationWork = "RETRY_OBSERVATION"

type ObservationRepository interface {
	LoadAttempt(context.Context, WorkFence) (AttemptSnapshot, error)
	CommitObservation(context.Context, WorkFence, ObservationDecision) error
}

type ObservationTarget interface {
	ObserveTask(context.Context, TaskObservationRequest) (Observation, error)
	ObserveInventory(context.Context, InventoryObservationRequest) (Observation, error)
	ObserveHealth(context.Context, HealthObservationRequest) (Observation, error)
}

// ObserveOnce obtains exactly one evidence document and lets the repository
// atomically persist it with the fenced lifecycle decision. It never submits
// firmware and it never turns a missing/contradictory observation into retry
// authority for a firmware request.
func ObserveOnce(ctx context.Context, repository ObservationRepository, target ObservationTarget, work WorkFence, workType string) error {
	if repository == nil || target == nil || work.DueWorkID == "" || work.CommandID == "" || work.LeaseToken == "" || work.ExpectedRevision < 0 {
		return fmt.Errorf("invalid observation execution request")
	}
	attempt, err := repository.LoadAttempt(ctx, work)
	if err != nil {
		return fmt.Errorf("load fenced observation attempt: %w", err)
	}
	var observation Observation
	switch workType {
	case "TASK_OBSERVE":
		observation, err = target.ObserveTask(ctx, TaskObservationRequest{Attempt: attempt})
	case "INVENTORY_OBSERVE":
		observation, err = target.ObserveInventory(ctx, InventoryObservationRequest{Attempt: attempt})
	case "HEALTH_OBSERVE":
		observation, err = target.ObserveHealth(ctx, HealthObservationRequest{Attempt: attempt})
	default:
		return fmt.Errorf("unsupported observation work type %q", workType)
	}
	if err != nil {
		return fmt.Errorf("collect %s evidence: %w", workType, err)
	}
	if observation.AttemptID != attempt.AttemptID || observation.TargetID != attempt.TargetID || observation.TargetUpdateID != attempt.TargetUpdateID || len(observation.Data) == 0 || observation.EvidenceFingerprint == "" || observation.ObservedAt.IsZero() || observation.ReceivedAt.IsZero() {
		return fmt.Errorf("invalid observation evidence")
	}
	decision, err := observationDecision(work, observation)
	if err != nil {
		return err
	}
	return repository.CommitObservation(ctx, work, decision)
}

func observationDecision(work WorkFence, observation Observation) (ObservationDecision, error) {
	next := retryObservationWork
	var document map[string]any
	if err := json.Unmarshal(observation.Data, &document); err != nil {
		return ObservationDecision{}, fmt.Errorf("decode normalized observation: %w", err)
	}
	switch observation.Kind {
	case ObservationTask:
		state, _ := document["TaskState"].(string)
		switch state {
		case "Completed":
			next = "INVENTORY_OBSERVE"
		case "Exception", "Killed":
			next = "TERMINAL"
		}
	case ObservationInventory:
		if !observation.ContradictsExpectedState {
			next = "HEALTH_OBSERVE"
		}
	case ObservationHealth:
		if observation.ContradictsExpectedState {
			next = "TERMINAL"
		} else {
			next = "SUCCEEDED"
		}
	default:
		return ObservationDecision{}, fmt.Errorf("unknown observation kind %q", observation.Kind)
	}
	return ObservationDecision{Work: work, Observation: observation, NextWorkType: next}, nil
}
