package localwakeup

import (
	"context"
	"github.com/parth10-05/fleetforge/internal/application/orchestration"
	"testing"
	"time"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }
func envelope() orchestration.Envelope {
	return orchestration.Envelope{SchemaVersion: orchestration.WakeupSchemaV1, QueueClass: orchestration.QueueControl, DueWorkID: "00000000-0000-4000-8000-000000000001", CommandID: "00000000-0000-4000-8000-000000000002", WorkType: "SAFETY_PROBE", SubjectType: "TARGET_UPDATE", SubjectID: "00000000-0000-4000-8000-000000000003", ExpectedRevision: 1, NotBefore: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)}
}
func TestFIFOAndAck(t *testing.T) {
	c := &clock{now: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)}
	q, _ := New(Config{Capacity: 2, Clock: c})
	e := envelope()
	if _, err := q.Publish(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	got, err := q.Receive(context.Background(), 1)
	if err != nil || len(got) != 1 {
		t.Fatalf("receive=%d %v", len(got), err)
	}
	if err := q.Ack(context.Background(), got[0]); err != nil {
		t.Fatal(err)
	}
	got, _ = q.Receive(context.Background(), 1)
	if len(got) != 0 {
		t.Fatal("ack did not remove receipt")
	}
}
func TestFaultsAreDeterministic(t *testing.T) {
	c := &clock{now: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)}
	q, _ := New(Config{Capacity: 4, Clock: c})
	q.SetFaults(Faults{Duplicate: true, Delay: time.Second, OutcomeUnknown: true})
	out, err := q.Publish(context.Background(), envelope())
	if err != nil || out != orchestration.PublishOutcomeUnknown {
		t.Fatalf("publish=%v %v", out, err)
	}
	got, _ := q.Receive(context.Background(), 4)
	if len(got) != 0 {
		t.Fatal("delayed receipt visible early")
	}
	c.now = c.now.Add(time.Second)
	got, _ = q.Receive(context.Background(), 4)
	if len(got) != 2 {
		t.Fatalf("duplicate count=%d", len(got))
	}
	q.SetFaults(Faults{AckFailure: true})
	if err := q.Ack(context.Background(), got[0]); err == nil {
		t.Fatal("ack failure missing")
	}
	q.SetFaults(Faults{})
	if err := q.Ack(context.Background(), got[0]); err != nil {
		t.Fatal(err)
	}
}

func TestPublishRejectsWhenCapacityIsExhausted(t *testing.T) {
	c := &clock{now: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)}
	q, err := New(Config{Capacity: 1, Clock: c})
	if err != nil {
		t.Fatal(err)
	}
	if outcome, err := q.Publish(context.Background(), envelope()); err != nil || outcome != orchestration.PublishAccepted {
		t.Fatalf("first publish = %v, %v", outcome, err)
	}
	if outcome, err := q.Publish(context.Background(), envelope()); err == nil || outcome != orchestration.PublishRejectedRetryable {
		t.Fatalf("full queue publish = %v, %v", outcome, err)
	}
}

func TestDropAndOutageDoNotEnqueue(t *testing.T) {
	c := &clock{now: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)}
	q, err := New(Config{Capacity: 1, Clock: c})
	if err != nil {
		t.Fatal(err)
	}
	q.SetFaults(Faults{Drop: true})
	if outcome, err := q.Publish(context.Background(), envelope()); err != nil || outcome != orchestration.PublishAccepted {
		t.Fatalf("dropped publish = %v, %v", outcome, err)
	}
	if got, err := q.Receive(context.Background(), 1); err != nil || len(got) != 0 {
		t.Fatalf("dropped receive = %d, %v", len(got), err)
	}
	q.SetFaults(Faults{Outage: true})
	if outcome, err := q.Publish(context.Background(), envelope()); err == nil || outcome != orchestration.PublishRejectedRetryable {
		t.Fatalf("outage publish = %v, %v", outcome, err)
	}
}

func TestCloseModelsProcessLocalLoss(t *testing.T) {
	c := &clock{now: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)}
	q, err := New(Config{Capacity: 1, Clock: c})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Publish(context.Background(), envelope()); err != nil {
		t.Fatal(err)
	}
	q.Close()
	if outcome, err := q.Publish(context.Background(), envelope()); err != ErrClosed || outcome != orchestration.PublishRejectedRetryable {
		t.Fatalf("closed publish = %v, %v", outcome, err)
	}
	if _, err := q.Receive(context.Background(), 1); err != ErrClosed {
		t.Fatalf("closed receive error = %v", err)
	}
}
