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
}

func (s *storeStub) Claim(context.Context, Envelope) (ClaimedWork, ClaimDisposition, error) {
	return ClaimedWork{DueWorkID: "work", LeaseToken: "lease"}, s.claim, nil
}
func (s *storeStub) Complete(context.Context, ClaimedWork) error { s.completed++; return s.completeErr }
func (s *storeStub) Quarantine(context.Context, Receipt, string) error {
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
func TestWorkerQuarantinesBeforeAcknowledgingInvalidReceipt(t *testing.T) {
	store, handler, acker := &storeStub{}, &handlerStub{}, &ackerStub{}
	if err := workerFor(t, Receipt{Raw: []byte("bad")}, store, handler, acker).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.quarantined != 1 || acker.count != 1 {
		t.Fatalf("quarantine=%d ack=%d", store.quarantined, acker.count)
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
