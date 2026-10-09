package postgres

import "testing"

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
