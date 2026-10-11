package execution

import (
	"context"
	"errors"
	"testing"
	"time"
)

type observationRepositoryStub struct{}

func (observationRepositoryStub) LoadAttempt(context.Context, WorkFence) (AttemptSnapshot, error) {
	return AttemptSnapshot{}, errors.New("must not load attempt")
}
func (observationRepositoryStub) CommitObservation(context.Context, WorkFence, ObservationDecision) error {
	return errors.New("must not commit observation")
}

type observationTargetStub struct{}

func (observationTargetStub) ObserveTask(context.Context, TaskObservationRequest) (Observation, error) {
	return Observation{}, nil
}
func (observationTargetStub) ObserveReboot(context.Context, RebootObservationRequest) (Observation, error) {
	return Observation{}, nil
}
func (observationTargetStub) ObserveInventory(context.Context, InventoryObservationRequest) (Observation, error) {
	return Observation{}, nil
}
func (observationTargetStub) ObserveHealth(context.Context, HealthObservationRequest) (Observation, error) {
	return Observation{}, nil
}

func TestObserveOnceRejectsInvalidDeliveryGenerationBeforeTargetRead(t *testing.T) {
	err := ObserveOnce(context.Background(), observationRepositoryStub{}, observationTargetStub{}, WorkFence{DueWorkID: "work", CommandID: "command", LeaseToken: "lease", ExpectedRevision: 0, DeliveryGeneration: -1}, "TASK_OBSERVE")
	if err == nil {
		t.Fatal("invalid delivery generation was accepted")
	}
}

func TestObservationDecisionRoutesOnlyEvidenceLifecycleSteps(t *testing.T) {
	now := time.Now().UTC()
	work := WorkFence{DueWorkID: "work", CommandID: "command", LeaseToken: "lease"}
	cases := []struct {
		name        string
		observation Observation
		want        string
		fresh       bool
	}{
		{"completed task", Observation{Kind: ObservationTask, Data: []byte(`{"TaskState":"Completed"}`), ObservedAt: now, ReceivedAt: now}, "REBOOT_OBSERVE", true},
		{"missing task defers to inventory", Observation{Kind: ObservationTask, Data: []byte(`{"TaskState":"Missing"}`), ObservedAt: now, ReceivedAt: now}, "INVENTORY_OBSERVE", true},
		{"running task", Observation{Kind: ObservationTask, Data: []byte(`{"TaskState":"Running"}`), ObservedAt: now, ReceivedAt: now}, retryObservationWork, true},
		{"failed task", Observation{Kind: ObservationTask, Data: []byte(`{"TaskState":"Exception"}`), ObservedAt: now, ReceivedAt: now}, "TERMINAL", true},
		{"reachable after reboot", Observation{Kind: ObservationReboot, Data: []byte(`{"Status":{"State":"Enabled"}}`), ObservedAt: now, ReceivedAt: now}, "INVENTORY_OBSERVE", true},
		{"matching inventory", Observation{Kind: ObservationInventory, Data: []byte(`{"Version":"2"}`), ObservedAt: now, ReceivedAt: now}, "HEALTH_OBSERVE", true},
		{"mismatching inventory", Observation{Kind: ObservationInventory, Data: []byte(`{"Version":"1"}`), ContradictsExpectedState: true, ObservedAt: now, ReceivedAt: now}, retryObservationWork, true},
		{"healthy", Observation{Kind: ObservationHealth, Data: []byte(`{"Status":{"Health":"OK"}}`), ObservedAt: now, ReceivedAt: now}, "SUCCEEDED", true},
		{"stale task evidence", Observation{Kind: ObservationTask, Data: []byte(`{"TaskState":"Completed"}`), ObservedAt: now, ReceivedAt: now}, retryObservationWork, false},
		{"stale reboot evidence", Observation{Kind: ObservationReboot, Data: []byte(`{"Status":{"State":"Enabled"}}`), ObservedAt: now, ReceivedAt: now}, retryObservationWork, false},
		{"stale inventory evidence", Observation{Kind: ObservationInventory, Data: []byte(`{"Version":"2"}`), ObservedAt: now, ReceivedAt: now}, retryObservationWork, false},
		{"stale healthy evidence", Observation{Kind: ObservationHealth, Data: []byte(`{"Status":{"Health":"OK"}}`), ObservedAt: now, ReceivedAt: now}, retryObservationWork, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.observation.Fresh = testCase.fresh
			decision, err := observationDecision(work, testCase.observation)
			if err != nil || decision.NextWorkType != testCase.want {
				t.Fatalf("decision=%+v err=%v", decision, err)
			}
		})
	}
}
