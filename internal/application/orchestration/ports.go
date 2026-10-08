package orchestration

import (
	"context"
	"time"
)

type PublishOutcome uint8

const (
	PublishAccepted PublishOutcome = iota
	PublishRejectedRetryable
	PublishRejectedPermanent
	PublishOutcomeUnknown
)

type Receipt struct {
	Raw    []byte
	Handle any
}
type WakeupPublisher interface {
	Publish(context.Context, Envelope) (PublishOutcome, error)
}
type WakeupReceiver interface {
	Receive(context.Context, int) ([]Receipt, error)
}
type WakeupAcknowledger interface {
	Ack(context.Context, Receipt) error
}
type Clock interface{ Now() time.Time }
type ClockFunc func() time.Time

func (f ClockFunc) Now() time.Time { return f() }
