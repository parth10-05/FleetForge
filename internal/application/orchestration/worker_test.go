package orchestration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type receiverStub struct{ receipts []Receipt }

func (s receiverStub) Receive(context.Context, int) ([]Receipt, error) { return s.receipts, nil }

type ackerStub struct {
	mu    sync.Mutex
	count int
	err   error
}

func (s *ackerStub) Ack(context.Context, Receipt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	return s.err
}

type storeStub struct {
	claim                      ClaimDisposition
	completeErr, quarantineErr error
	completed, quarantined     int
	quarantineReason           string
}

func (s *storeStub) Claim(context.Context, Envelope) (ClaimedWork, ClaimDisposition, error) {
	return ClaimedWork{DueWorkID: "work", LeaseToken: "lease"}, s.claim, nil
}
func (s *storeStub) Complete(context.Context, ClaimedWork) error { s.completed++; return s.completeErr }
func (s *storeStub) Quarantine(_ context.Context, _ Receipt, reason string) error {
	// The reason is part of the durable forensic contract: worker tests retain
	// it to make sure invalid forms are not collapsed into a generic error.
	//nolint:unparam // interface signature is intentionally production-shaped.
	s.quarantineReason = reason
	s.quarantined++
	return s.quarantineErr
}

type handlerStub struct {
	supported bool
	err       error
	calls     int
}

func (s *handlerStub) Supports(Envelope) bool                    { return s.supported }
func (s *handlerStub) Handle(context.Context, ClaimedWork) error { s.calls++; return s.err }

type finalizedHandlerStub struct {
	supported bool
	err       error
	calls     int
}

func (s *finalizedHandlerStub) Supports(Envelope) bool { return s.supported }
func (s *finalizedHandlerStub) HandleFinalized(context.Context, ClaimedWork) error {
	s.calls++
	return s.err
}

func validReceipt(t *testing.T) Receipt {
	t.Helper()
	raw, err := Encode(testEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	return Receipt{Raw: raw}
}
func workerFor(t *testing.T, receipt Receipt, store *storeStub, handler *handlerStub, acker *ackerStub) *Worker {
	t.Helper()
	worker, err := NewWorker(receiverStub{receipts: []Receipt{receipt}}, acker, store, handler, WorkerConfig{ReceiveBatch: 1, Concurrency: 1, HandlerTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return worker
}

func TestWorkerCompletesBeforeAcknowledging(t *testing.T) {
	store, handler, acker := &storeStub{claim: ClaimOwned}, &handlerStub{supported: true}, &ackerStub{}
	if err := workerFor(t, validReceipt(t), store, handler, acker).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.completed != 1 || handler.calls != 1 || acker.count != 1 {
		t.Fatalf("complete=%d handle=%d ack=%d", store.completed, handler.calls, acker.count)
	}
}
func TestWorkerDoesNotAcknowledgeAmbiguousCompletion(t *testing.T) {
	store, handler, acker := &storeStub{claim: ClaimOwned, completeErr: errors.New("database unavailable")}, &handlerStub{supported: true}, &ackerStub{}
	if err := workerFor(t, validReceipt(t), store, handler, acker).RunOnce(context.Background()); err == nil {
		t.Fatal("missing completion error")
	}
	if acker.count != 0 {
		t.Fatalf("ack=%d, want 0", acker.count)
	}
}

func TestWorkerAcknowledgesFinalizedHandlerWithoutGenericCompletion(t *testing.T) {
	store, handler, acker := &storeStub{claim: ClaimOwned}, &finalizedHandlerStub{supported: true}, &ackerStub{}
	worker, err := NewWorker(receiverStub{receipts: []Receipt{validReceipt(t)}}, acker, store, handler, WorkerConfig{ReceiveBatch: 1, Concurrency: 1, HandlerTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.completed != 0 || handler.calls != 1 || acker.count != 1 {
		t.Fatalf("complete=%d finalized=%d ack=%d", store.completed, handler.calls, acker.count)
	}
}

func TestWorkerRecordsFinalizedDurableCompletion(t *testing.T) {
	store, handler, acker := &storeStub{claim: ClaimOwned}, &finalizedHandlerStub{supported: true}, &ackerStub{}
	metrics := NewMetrics()
	worker, err := NewWorker(receiverStub{receipts: []Receipt{validReceipt(t)}}, acker, store, handler, WorkerConfig{ReceiveBatch: 1, Concurrency: 1, HandlerTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.WithMetrics(metrics).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if metrics.Snapshot()[MetricKey{Name: "durable_complete", Outcome: "success", Class: "control"}] != 1 {
		t.Fatalf("missing finalized completion metric: %#v", metrics.Snapshot())
	}
}

func TestWorkerLeavesReceiptWhenFinalizedHandlerFails(t *testing.T) {
	store, handler, acker := &storeStub{claim: ClaimOwned}, &finalizedHandlerStub{supported: true, err: errors.New("commit failed")}, &ackerStub{}
	worker, err := NewWorker(receiverStub{receipts: []Receipt{validReceipt(t)}}, acker, store, handler, WorkerConfig{ReceiveBatch: 1, Concurrency: 1, HandlerTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("missing finalization error")
	}
	if store.completed != 0 || acker.count != 0 {
		t.Fatalf("complete=%d ack=%d", store.completed, acker.count)
	}
}
func TestWorkerQuarantinesBeforeAcknowledgingInvalidReceipt(t *testing.T) {
	store, handler, acker := &storeStub{}, &handlerStub{}, &ackerStub{}
	if err := workerFor(t, Receipt{Raw: []byte("bad")}, store, handler, acker).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.quarantined != 1 || acker.count != 1 {
		t.Fatalf("quarantine=%d ack=%d", store.quarantined, acker.count)
	}
	if store.quarantineReason != QuarantineMalformedEncoding {
		t.Fatalf("quarantine reason=%q", store.quarantineReason)
	}
}
func TestWorkerAcknowledgesAuthoritativelyIgnoredDelivery(t *testing.T) {
	store, handler, acker := &storeStub{claim: ClaimIgnored}, &handlerStub{supported: true}, &ackerStub{}
	if err := workerFor(t, validReceipt(t), store, handler, acker).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if handler.calls != 0 || store.completed != 0 || acker.count != 1 {
		t.Fatalf("handle=%d complete=%d ack=%d", handler.calls, store.completed, acker.count)
	}
}

func TestSafetyProbeHandlerAllowsNoDestructiveWork(t *testing.T) {
	handler := SafetyProbeHandler{}
	probe := testEnvelope()
	probe.WorkType, probe.QueueClass = "SAFETY_PROBE", QueueControl
	if !handler.Supports(probe) {
		t.Fatal("safety probe should be supported")
	}
	for _, workType := range []string{"SUBMIT", "UPDATE_STEP", "RED FISH"} {
		probe.WorkType = workType
		if handler.Supports(probe) {
			t.Fatalf("unsupported work type %q accepted", workType)
		}
	}
	if err := handler.Handle(context.Background(), ClaimedWork{}); err != nil {
		t.Fatal(err)
	}
}

type concurrentStore struct{}

func (concurrentStore) Claim(context.Context, Envelope) (ClaimedWork, ClaimDisposition, error) {
	return ClaimedWork{DueWorkID: "work", LeaseToken: "lease"}, ClaimOwned, nil
}
func (concurrentStore) Complete(context.Context, ClaimedWork) error       { return nil }
func (concurrentStore) Quarantine(context.Context, Receipt, string) error { return nil }

type concurrentHandler struct {
	mu           sync.Mutex
	active, peak int
	entered      chan struct{}
	release      <-chan struct{}
}

func (h *concurrentHandler) Supports(Envelope) bool { return true }
func (h *concurrentHandler) Handle(context.Context, ClaimedWork) error {
	h.mu.Lock()
	h.active++
	if h.active > h.peak {
		h.peak = h.active
	}
	h.mu.Unlock()
	h.entered <- struct{}{}
	<-h.release
	h.mu.Lock()
	h.active--
	h.mu.Unlock()
	return nil
}

func TestWorkerBoundsConcurrentDispatch(t *testing.T) {
	first, second := validReceipt(t), validReceipt(t)
	release := make(chan struct{})
	handler := &concurrentHandler{entered: make(chan struct{}, 2), release: release}
	acker := &ackerStub{}
	worker, err := NewWorker(receiverStub{receipts: []Receipt{first, second}}, acker, concurrentStore{}, handler, WorkerConfig{ReceiveBatch: 2, Concurrency: 1, HandlerTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- worker.RunOnce(context.Background()) }()
	select {
	case <-handler.entered:
	case <-time.After(time.Second):
		t.Fatal("first handler did not start")
	}
	select {
	case <-handler.entered:
		t.Fatal("worker exceeded concurrency")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if handler.peak != 1 || acker.count != 2 {
		t.Fatalf("peak=%d ack=%d", handler.peak, acker.count)
	}
}

type cancellationHandler struct {
	entered chan struct{}
	calls   int
}

func (h *cancellationHandler) Supports(Envelope) bool { return true }
func (h *cancellationHandler) Handle(ctx context.Context, _ ClaimedWork) error {
	h.calls++
	h.entered <- struct{}{}
	<-ctx.Done()
	return ctx.Err()
}

func TestWorkerStopsSchedulingAndJoinsHandlersOnCancellation(t *testing.T) {
	first, second := validReceipt(t), validReceipt(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handler := &cancellationHandler{entered: make(chan struct{}, 1)}
	acker := &ackerStub{}
	worker, err := NewWorker(receiverStub{receipts: []Receipt{first, second}}, acker, concurrentStore{}, handler, WorkerConfig{ReceiveBatch: 2, Concurrency: 1, HandlerTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- worker.RunOnce(ctx) }()
	select {
	case <-handler.entered:
	case <-time.After(time.Second):
		t.Fatal("first handler did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunOnce() error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not join cancelled handler")
	}
	if handler.calls != 1 || acker.count != 0 {
		t.Fatalf("calls=%d ack=%d, want one unacknowledged in-flight receipt", handler.calls, acker.count)
	}
}

func TestWorkerRecordsBoundedOutcomes(t *testing.T) {
	store, handler, acker := &storeStub{claim: ClaimOwned}, &handlerStub{supported: true}, &ackerStub{}
	metrics := NewMetrics()
	worker := workerFor(t, validReceipt(t), store, handler, acker).WithMetrics(metrics)
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, key := range []MetricKey{{Name: "receive", Outcome: "success"}, {Name: "claim", Outcome: "success", Class: "control"}, {Name: "dispatch", Outcome: "success", Class: "control"}, {Name: "durable_complete", Outcome: "success", Class: "control"}, {Name: "ack", Outcome: "success", Class: "control"}} {
		if metrics.Snapshot()[key] != 1 {
			t.Fatalf("metric %#v = %d", key, metrics.Snapshot()[key])
		}
	}
}
