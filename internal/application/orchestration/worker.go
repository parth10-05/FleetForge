package orchestration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type ClaimDisposition uint8

const (
	ClaimIgnored ClaimDisposition = iota
	ClaimOwned
)

// ClaimedWork is deliberately transport-free. A handler receives only durable
// work identity and a fencing token; it cannot mistake a receipt handle for
// command authority.
type ClaimedWork struct {
	DueWorkID, CommandID, LeaseToken string
	WorkType                         string
	DeliveryGeneration               int64
	ExpectedRevision                 int64
	ExpectedCommandEpoch             *int64
}

type WorkStore interface {
	Claim(context.Context, Envelope) (ClaimedWork, ClaimDisposition, error)
	Complete(context.Context, ClaimedWork) error
	Quarantine(context.Context, Receipt, string) error
}

type WorkHandler interface {
	Supports(Envelope) bool
	Handle(context.Context, ClaimedWork) error
}

type WorkerConfig struct {
	ReceiveBatch, Concurrency int
	HandlerTimeout            time.Duration
}

type Worker struct {
	receiver WakeupReceiver
	acker    WakeupAcknowledger
	store    WorkStore
	handler  WorkHandler
	config   WorkerConfig
	metrics  *Metrics
}

func (w *Worker) WithMetrics(metrics *Metrics) *Worker { w.metrics = metrics; return w }

func NewWorker(receiver WakeupReceiver, acker WakeupAcknowledger, store WorkStore, handler WorkHandler, config WorkerConfig) (*Worker, error) {
	if receiver == nil || acker == nil || store == nil || handler == nil || config.ReceiveBatch <= 0 || config.ReceiveBatch > 100 || config.Concurrency <= 0 || config.Concurrency > config.ReceiveBatch || config.HandlerTimeout <= 0 {
		return nil, fmt.Errorf("invalid wakeup worker configuration")
	}
	return &Worker{receiver: receiver, acker: acker, store: store, handler: handler, config: config}, nil
}

// RunOnce receives one bounded batch. A receipt is acknowledged only after a
// durable terminal classification or durable completion; ambiguous failures
// deliberately leave it available for at-least-once redelivery.
func (w *Worker) RunOnce(ctx context.Context) error {
	receipts, err := w.receiver.Receive(ctx, w.config.ReceiveBatch)
	if err != nil {
		w.record("receive", "failure", "")
		return err
	}
	w.record("receive", "success", "")
	sem := make(chan struct{}, w.config.Concurrency)
	errs := make(chan error, len(receipts))
	var group sync.WaitGroup
	for _, receipt := range receipts {
		if err := ctx.Err(); err != nil {
			return err
		}
		sem <- struct{}{}
		group.Add(1)
		go func(receipt Receipt) {
			defer group.Done()
			defer func() { <-sem }()
			if err := w.process(ctx, receipt); err != nil {
				errs <- err
			}
		}(receipt)
	}
	group.Wait()
	close(errs)
	var joined error
	for err := range errs {
		joined = errors.Join(joined, err)
	}
	return joined
}

func (w *Worker) process(ctx context.Context, receipt Receipt) error {
	envelope, err := Decode(receipt.Raw)
	if err != nil {
		w.record("quarantine", "success", "")
		return w.quarantineThenAck(ctx, receipt, "MALFORMED_ENCODING")
	}
	if !w.handler.Supports(envelope) {
		w.record("quarantine", "success", string(envelope.QueueClass))
		return w.quarantineThenAck(ctx, receipt, "UNSUPPORTED_WORK_TYPE")
	}
	claimed, disposition, err := w.store.Claim(ctx, envelope)
	if err != nil {
		w.record("claim", "failure", string(envelope.QueueClass))
		return err
	}
	if disposition == ClaimIgnored {
		w.record("claim", "stale", string(envelope.QueueClass))
		return w.acker.Ack(ctx, receipt)
	}
	w.record("claim", "success", string(envelope.QueueClass))
	handlerContext, cancel := context.WithTimeout(ctx, w.config.HandlerTimeout)
	err = w.handler.Handle(handlerContext, claimed)
	cancel()
	if err != nil {
		w.record("dispatch", "failure", string(envelope.QueueClass))
		return err
	}
	w.record("dispatch", "success", string(envelope.QueueClass))
	if err := w.store.Complete(ctx, claimed); err != nil {
		w.record("durable_complete", "failure", string(envelope.QueueClass))
		return err
	}
	w.record("durable_complete", "success", string(envelope.QueueClass))
	err = w.acker.Ack(ctx, receipt)
	if err != nil {
		w.record("ack", "failure", string(envelope.QueueClass))
		return err
	}
	w.record("ack", "success", string(envelope.QueueClass))
	return nil
}

func (w *Worker) record(name, outcome, class string) {
	if w.metrics != nil {
		_ = w.metrics.Record(name, outcome, class)
	}
}

func (w *Worker) quarantineThenAck(ctx context.Context, receipt Receipt, reason string) error {
	if err := w.store.Quarantine(ctx, receipt, reason); err != nil {
		return err
	}
	return w.acker.Ack(ctx, receipt)
}
