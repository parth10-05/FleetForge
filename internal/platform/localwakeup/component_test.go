package localwakeup

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/orchestration"
)

type componentClock struct{ now time.Time }

func (c componentClock) Now() time.Time { return c.now }

type componentStore struct {
	mu          sync.Mutex
	claimed     bool
	completions int
}

func (s *componentStore) Claim(context.Context, orchestration.Envelope) (orchestration.ClaimedWork, orchestration.ClaimDisposition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed {
		return orchestration.ClaimedWork{}, orchestration.ClaimIgnored, nil
	}
	s.claimed = true
	return orchestration.ClaimedWork{DueWorkID: "work", LeaseToken: "lease", WorkType: "SAFETY_PROBE"}, orchestration.ClaimOwned, nil
}
func (s *componentStore) Complete(context.Context, orchestration.ClaimedWork) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completions++
	return nil
}
func (s *componentStore) Quarantine(context.Context, orchestration.Receipt, string) error { return nil }

func TestDuplicateWakeupComponentCompletesOnceWithoutDestructiveHandler(t *testing.T) {
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	queue, err := New(Config{Capacity: 4, Clock: componentClock{now}})
	if err != nil {
		t.Fatal(err)
	}
	queue.SetFaults(Faults{Duplicate: true})
	envelope := orchestration.Envelope{SchemaVersion: orchestration.WakeupSchemaV1, QueueClass: orchestration.QueueControl, DueWorkID: "00000000-0000-4000-8000-000000000001", CommandID: "00000000-0000-4000-8000-000000000002", WorkType: "SAFETY_PROBE", SubjectType: "TARGET_UPDATE", SubjectID: "00000000-0000-4000-8000-000000000003", ExpectedRevision: 1, NotBefore: now}
	if _, err := queue.Publish(context.Background(), envelope); err != nil {
		t.Fatal(err)
	}
	store := &componentStore{}
	worker, err := orchestration.NewWorker(queue, queue, store, orchestration.SafetyProbeHandler{}, orchestration.WorkerConfig{ReceiveBatch: 4, Concurrency: 2, HandlerTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.completions != 1 {
		t.Fatalf("durable completions=%d, want 1", store.completions)
	}
	if remaining, err := queue.Receive(context.Background(), 4); err != nil || len(remaining) != 0 {
		t.Fatalf("remaining=%d err=%v", len(remaining), err)
	}
}
