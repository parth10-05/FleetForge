package orchestration

import (
	"context"
	"fmt"
)

// MultiplexHandler routes each authenticated wakeup to exactly one handler.
// Finalizing handlers own their transaction; ordinary handlers retain the
// worker's generic completion path.
type MultiplexHandler struct {
	ordinary   []WorkHandler
	finalizing []FinalizingWorkHandler
}

func NewMultiplexHandler(ordinary []WorkHandler, finalizing []FinalizingWorkHandler) (*MultiplexHandler, error) {
	if len(ordinary)+len(finalizing) == 0 {
		return nil, fmt.Errorf("empty wakeup handler set")
	}
	for _, handler := range ordinary {
		if handler == nil {
			return nil, fmt.Errorf("nil ordinary wakeup handler")
		}
	}
	for _, handler := range finalizing {
		if handler == nil {
			return nil, fmt.Errorf("nil finalizing wakeup handler")
		}
	}
	return &MultiplexHandler{ordinary: append([]WorkHandler(nil), ordinary...), finalizing: append([]FinalizingWorkHandler(nil), finalizing...)}, nil
}

func (handler *MultiplexHandler) Supports(envelope Envelope) bool {
	return handler.matchOrdinary(envelope) != nil || handler.matchFinalizing(envelope) != nil
}
func (handler *MultiplexHandler) Finalizes(envelope Envelope) bool {
	return handler.matchFinalizing(envelope) != nil
}
func (handler *MultiplexHandler) Handle(ctx context.Context, work ClaimedWork) error {
	for _, candidate := range handler.ordinary {
		if candidate.Supports(Envelope{QueueClass: work.QueueClass, WorkType: work.WorkType}) {
			return candidate.Handle(ctx, work)
		}
	}
	return fmt.Errorf("no ordinary wakeup handler for %s", work.WorkType)
}
func (handler *MultiplexHandler) HandleFinalized(ctx context.Context, work ClaimedWork) error {
	for _, candidate := range handler.finalizing {
		if candidate.Supports(Envelope{QueueClass: work.QueueClass, WorkType: work.WorkType}) {
			return candidate.HandleFinalized(ctx, work)
		}
	}
	return fmt.Errorf("no finalizing wakeup handler for %s", work.WorkType)
}
func (handler *MultiplexHandler) matchOrdinary(envelope Envelope) WorkHandler {
	for _, candidate := range handler.ordinary {
		if candidate.Supports(envelope) {
			return candidate
		}
	}
	return nil
}
func (handler *MultiplexHandler) matchFinalizing(envelope Envelope) FinalizingWorkHandler {
	for _, candidate := range handler.finalizing {
		if candidate.Supports(envelope) {
			return candidate
		}
	}
	return nil
}
