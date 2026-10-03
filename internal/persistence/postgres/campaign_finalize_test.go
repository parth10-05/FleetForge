package postgres

import "testing"

func TestNullableStatePreservesOnlyReducerSuspension(t *testing.T) {
	if got := nullableState(""); got != nil {
		t.Fatalf("empty suspended phase = %#v, want nil", got)
	}
	if got := nullableState("RUNNING_CANARY"); got != "RUNNING_CANARY" {
		t.Fatalf("running suspended phase = %#v", got)
	}
}
