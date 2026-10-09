package execution

import (
	"testing"
	"time"
)

func TestObservationDecisionRoutesOnlyEvidenceLifecycleSteps(t *testing.T) {
	now := time.Now().UTC()
	work := WorkFence{DueWorkID: "work", CommandID: "command", LeaseToken: "lease"}
	cases := []struct {
		name        string
		observation Observation
		want        string
	}{
		{"completed task", Observation{Kind: ObservationTask, Data: []byte(`{"TaskState":"Completed"}`), ObservedAt: now, ReceivedAt: now}, "INVENTORY_OBSERVE"},
		{"running task", Observation{Kind: ObservationTask, Data: []byte(`{"TaskState":"Running"}`), ObservedAt: now, ReceivedAt: now}, retryObservationWork},
		{"failed task", Observation{Kind: ObservationTask, Data: []byte(`{"TaskState":"Exception"}`), ObservedAt: now, ReceivedAt: now}, "TERMINAL"},
		{"matching inventory", Observation{Kind: ObservationInventory, Data: []byte(`{"Version":"2"}`), ObservedAt: now, ReceivedAt: now}, "HEALTH_OBSERVE"},
		{"mismatching inventory", Observation{Kind: ObservationInventory, Data: []byte(`{"Version":"1"}`), ContradictsExpectedState: true, ObservedAt: now, ReceivedAt: now}, retryObservationWork},
		{"healthy", Observation{Kind: ObservationHealth, Data: []byte(`{"Status":{"Health":"OK"}}`), ObservedAt: now, ReceivedAt: now}, "SUCCEEDED"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			decision, err := observationDecision(work, testCase.observation)
			if err != nil || decision.NextWorkType != testCase.want {
				t.Fatalf("decision=%+v err=%v", decision, err)
			}
		})
	}
}
