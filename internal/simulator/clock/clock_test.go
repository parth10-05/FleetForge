package clock

import (
	"errors"
	"reflect"
	"testing"
)

func TestManualClockAndDeterministicTimerOrdering(t *testing.T) {
	manual := NewManual(5)
	if err := manual.AdvanceBy(7); err != nil || manual.Now() != 12 {
		t.Fatalf("AdvanceBy() = %d, %v", manual.Now(), err)
	}
	if err := manual.AdvanceTo(11); !errors.Is(err, ErrTimeReversal) {
		t.Fatalf("reverse error = %v", err)
	}
	var scheduler Scheduler
	scheduler.Schedule(20, 1, "target-b", "later-priority")
	scheduler.Schedule(20, 0, "target-b", "priority")
	scheduler.Schedule(20, 0, "target-a", "target")
	scheduler.Schedule(20, 0, "target-a", "sequence")
	values := []string{}
	for _, event := range scheduler.PopDue(20) {
		values = append(values, event.Value)
	}
	if want := []string{"target", "sequence", "priority", "later-priority"}; !reflect.DeepEqual(values, want) {
		t.Fatalf("order = %#v, want %#v", values, want)
	}
}
