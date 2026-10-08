package orchestration

import "context"

// SafetyProbeHandler is the only V1-P04 built-in execution handler. It proves
// the durable orchestration path with a no-op, simulator-safe completion; it
// intentionally has no Redfish, firmware, or admission dependency.
type SafetyProbeHandler struct{}

func (SafetyProbeHandler) Supports(envelope Envelope) bool {
	return envelope.QueueClass == QueueControl && envelope.WorkType == "SAFETY_PROBE"
}

func (SafetyProbeHandler) Handle(context.Context, ClaimedWork) error { return nil }
