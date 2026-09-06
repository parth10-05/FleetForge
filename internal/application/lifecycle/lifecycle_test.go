package lifecycle

import (
	"context"
	"testing"
	"time"
)

func TestDrainStopsNewWorkAndWaitsForAcceptedWork(t *testing.T) {
	controller := NewController()
	release, accepted := controller.Acquire()
	if !accepted {
		t.Fatal("Acquire() accepted = false, want true")
	}
	controller.BeginDrain()
	if _, accepted = controller.Acquire(); accepted {
		t.Fatal("controller accepted new work after drain began")
	}
	drained := make(chan bool, 1)
	go func() { drained <- controller.Drain(context.Background()) }()
	select {
	case <-drained:
		t.Fatal("Drain() returned before release")
	default:
	}
	release()
	select {
	case got := <-drained:
		if !got {
			t.Fatal("Drain() = false, want true")
		}
	case <-time.After(time.Second):
		t.Fatal("Drain() did not finish")
	}
}

func TestDrainHonorsDeadline(t *testing.T) {
	controller := NewController()
	_, accepted := controller.Acquire()
	if !accepted {
		t.Fatal("Acquire() accepted = false, want true")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if controller.Drain(ctx) {
		t.Fatal("Drain() = true, want false after deadline")
	}
}
