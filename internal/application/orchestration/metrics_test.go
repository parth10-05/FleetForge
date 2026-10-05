package orchestration

import "testing"

func TestMetricsAcceptOnlyBoundedLabels(t *testing.T) {
	metrics := NewMetrics()
	if err := metrics.Record("publish", "unknown", "control"); err != nil {
		t.Fatal(err)
	}
	if err := metrics.Record("publish", "unknown", "control"); err != nil {
		t.Fatal(err)
	}
	if got := metrics.Snapshot()[MetricKey{Name: "publish", Outcome: "unknown", Class: "control"}]; got != 2 {
		t.Fatalf("count=%d", got)
	}
	for _, input := range [][3]string{{"publish", "unknown", "rack-42"}, {"publish", "command-123", "control"}, {"target-1", "success", "control"}} {
		if err := metrics.Record(input[0], input[1], input[2]); err == nil {
			t.Fatalf("dynamic metric accepted: %#v", input)
		}
	}
}
