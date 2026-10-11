package postgres

import (
	"testing"
	"time"
)

func TestObservationRoutesRespectSeparatedRebootAndVersionStages(t *testing.T) {
	cases := []struct {
		workType, state, next string
		valid                 bool
	}{
		{"TASK_OBSERVE", "UPDATE_ACCEPTED", "REBOOT_OBSERVE", true},
		{"REBOOT_OBSERVE", "WAITING_REBOOT", "INVENTORY_OBSERVE", true},
		{"INVENTORY_OBSERVE", "VERIFYING", "HEALTH_OBSERVE", true},
		{"INVENTORY_OBSERVE", "WAITING_REBOOT", "HEALTH_OBSERVE", false},
		{"HEALTH_OBSERVE", "VERIFYING", "SUCCEEDED", true},
	}
	for _, testCase := range cases {
		if got := validObservationRoute(testCase.workType, testCase.state, testCase.next); got != testCase.valid {
			t.Fatalf("validObservationRoute(%q,%q,%q)=%t, want %t", testCase.workType, testCase.state, testCase.next, got, testCase.valid)
		}
	}
}

func TestObservationDelayIsStableAndBounded(t *testing.T) {
	first := observationDelay("attempt-a:TASK_OBSERVE", 2)
	if again := observationDelay("attempt-a:TASK_OBSERVE", 2); again != first {
		t.Fatalf("observation delay must be stable: %s != %s", first, again)
	}
	if first < 5*time.Second || first > 10*time.Second {
		t.Fatalf("observation delay %s is outside [5s,10s]", first)
	}
	if next := observationDelay("attempt-a:TASK_OBSERVE", 3); next == first {
		t.Fatalf("generation must contribute to durable retry jitter")
	}
}
