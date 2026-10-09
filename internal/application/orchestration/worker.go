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

// FinalizingWorkHandler owns the entire fenced durable transition for a unit
// of work. It is reserved for handlers whose domain transaction both records
// the outcome and releases the lease; the wakeup worker must not issue a
// second generic Complete after HandleFinalized returns.
//
// Keeping this separate from WorkHandler preserves the original completion
// path for simple handlers and makes finalization ownership explicit at the
// composition boundary.
type FinalizingWorkHandler interface {
	Supports(Envelope) bool
	HandleFinalized(context.Context, ClaimedWork) error
}

// RoutedWorkHandler declares whether a handler owns a decoded wakeup. It is
// shared by ordinary and self-finalizing handler contracts.
type RoutedWorkHandler interface {
	Supports(Envelope) bool
}

type WorkerConfig struct {
	ReceiveBatch, Concurrency int
	HandlerTimeout            time.Duration
}

type Worker struct {
	receiver WakeupReceiver
	acker    WakeupAcknowledger
	store    WorkStore
	handler  RoutedWorkHandler
	config   WorkerConfig
	metrics  *Metrics
}

func (w *Worker) WithMetrics(metrics *Metrics) *Worker { w.metrics = metrics; return w }

func NewWorker(receiver WakeupReceiver, acker WakeupAcknowledger, store WorkStore, handler RoutedWorkHandler, config WorkerConfig) (*Worker, error) {
	if receiver == nil || acker == nil || store == nil || handler == nil || config.ReceiveBatch <= 0 || config.ReceiveBatch > 100 || config.Concurrency <= 0 || config.Concurrency > config.ReceiveBatch || config.HandlerTimeout <= 0 {
		return nil, fmt.Errorf("invalid wakeup worker configuration")
	}
	if _, ordinary := handler.(WorkHandler); !ordinary {
		if _, finalizing := handler.(FinalizingWorkHandler); !finalizing {
			return nil, fmt.Errorf("wakeup handler must implement a work execution contract")
		}
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
	stopped := false
	for _, receipt := range receipts {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			stopped = true
		}
		if stopped {
			break
		}
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
	if err := ctx.Err(); err != nil {
		joined = errors.Join(joined, err)
	}
	return joined
}

func (w *Worker) process(ctx context.Context, receipt Receipt) error {
	envelope, err := Decode(receipt.Raw)
	if err != nil {
		w.record("quarantine", "success", "")
		return w.quarantineThenAck(ctx, receipt, ClassifyEnvelopeFailure(receipt.Raw))
	}
	if !w.handler.Supports(envelope) {
		w.record("quarantine", "success", string(envelope.QueueClass))
		return w.quarantineThenAck(ctx, receipt, QuarantineUnsupportedWorkType)
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
	finalized := false
	switch handler := w.handler.(type) {
	case FinalizingWorkHandler:
		finalized = true
		err = handler.HandleFinalized(handlerContext, claimed)
	case WorkHandler:
		err = handler.Handle(handlerContext, claimed)
	default:
		err = fmt.Errorf("wakeup handler has no execution contract")
	}
	cancel()
	if err != nil {
		w.record("dispatch", "failure", string(envelope.QueueClass))
		return err
	}
	w.record("dispatch", "success", string(envelope.QueueClass))
	if !finalized {
		if err := w.store.Complete(ctx, claimed); err != nil {
			w.record("durable_complete", "failure", string(envelope.QueueClass))
			return err
		}
		w.record("durable_complete", "success", string(envelope.QueueClass))
	} else {
		w.record("durable_complete", "success", string(envelope.QueueClass))
	}
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
