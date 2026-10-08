package postgres

import (
	"testing"
	"time"
)

func TestValidWakeupTelemetryRejectsUnsafeSnapshot(t *testing.T) {
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	if !validWakeupTelemetry(WakeupTelemetrySnapshot{ObservedAt: now}) {
		t.Fatal("zero-valued bounded telemetry rejected")
	}
	if validWakeupTelemetry(WakeupTelemetrySnapshot{ObservedAt: now, OpenQuarantineCount: -1}) {
		t.Fatal("negative telemetry accepted")
	}
	if validWakeupTelemetry(WakeupTelemetrySnapshot{ObservedAt: now.In(time.FixedZone("offset", 3600))}) {
		t.Fatal("non-UTC telemetry timestamp accepted")
	}
}
