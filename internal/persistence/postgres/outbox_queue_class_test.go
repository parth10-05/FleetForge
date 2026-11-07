package postgres

import (
	"testing"

	"github.com/parth10-05/fleetforge/internal/application/orchestration"
)

func TestQueueClassForWorkTypeRoutesCampaignStartsToUpdateStep(t *testing.T) {
	for _, workType := range []string{"CANARY_START", "LATER_WAVE_START"} {
		if got := queueClassForWorkType(workType); got != string(orchestration.QueueUpdateStep) {
			t.Fatalf("%s queue class=%q, want %q", workType, got, orchestration.QueueUpdateStep)
		}
	}
}
