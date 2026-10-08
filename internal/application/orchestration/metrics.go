package orchestration

import (
	"fmt"
	"sync"
)

// MetricKey intentionally exposes only finite vocabularies. Callers may not
// attach campaign, target, command, site, rack, trace, or receipt identifiers.
type MetricKey struct{ Name, Outcome, Class string }

type Metrics struct {
	mu     sync.Mutex
	values map[MetricKey]uint64
}

func NewMetrics() *Metrics { return &Metrics{values: make(map[MetricKey]uint64)} }

func (m *Metrics) Record(name, outcome, class string) error {
	if m == nil || !validMetric(name, outcome, class) {
		return fmt.Errorf("invalid bounded metric")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[MetricKey{Name: name, Outcome: outcome, Class: class}]++
	return nil
}

func (m *Metrics) Snapshot() map[MetricKey]uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[MetricKey]uint64, len(m.values))
	for key, value := range m.values {
		out[key] = value
	}
	return out
}

func validMetric(name, outcome, class string) bool {
	if class != "" && class != "control" && class != "discovery" && class != "artifact-verification" && class != "update-step" && class != "reconciliation" && class != "GLOBAL" && class != "SITE" && class != "RACK" {
		return false
	}
	switch name {
	case "claim", "publish", "receive", "dispatch", "durable_complete", "ack", "retry", "quarantine", "repair", "lease_expiry", "stale_token", "capacity_wait", "queue_outage", "worker_saturation", "queue_depth":
	default:
		return false
	}
	switch outcome {
	case "accepted", "rejected", "retryable", "permanent", "unknown", "success", "failure", "stale", "exhausted", "open", "closed":
		return true
	}
	return false
}
