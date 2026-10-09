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
		fresh       bool
	}{
		{"completed task", Observation{Kind: ObservationTask, Data: []byte(`{"TaskState":"Completed"}`), ObservedAt: now, ReceivedAt: now}, "REBOOT_OBSERVE", true},
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
