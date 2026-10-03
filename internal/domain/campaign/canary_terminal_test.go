package campaign

import (
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestReduceTerminalCanaryPreservesEvaluatingState(t *testing.T) {
	revision, _ := value.NewRevision(4)
	epoch, _ := value.NewCommandEpoch(2)
	first, _ := value.ParseCommandID("00000000-0000-0000-0000-000000000101")
	second, _ := value.ParseCommandID("00000000-0000-0000-0000-000000000102")
	result, ok := ReduceTerminalCanary(Aggregate{State: StateRunningCanary, Revision: revision, CommandEpoch: epoch}, first, second, true)
	if !ok || result.Terminal.Aggregate.State != StateEvaluatingCanary || result.Resolved.Aggregate.State != StateRunningWaves {
		t.Fatalf("release transition = %#v, ok=%t", result, ok)
	}
	result, ok = ReduceTerminalCanary(Aggregate{State: StateRunningCanary, Revision: revision, CommandEpoch: epoch}, first, second, false)
	if !ok || result.Terminal.Aggregate.State != StateEvaluatingCanary || result.Resolved.Aggregate.State != StatePaused || result.Resolved.Aggregate.CommandEpoch != epoch+1 {
		t.Fatalf("pause transition = %#v, ok=%t", result, ok)
	}
}
