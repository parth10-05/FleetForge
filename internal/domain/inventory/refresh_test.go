package inventory

import (
	"errors"
	"testing"
	"time"
)

func TestNextRefreshIsStableAndBounded(t *testing.T) {
	now := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	interval := 10 * time.Minute
	first, err := NextRefresh("rack-a-bmc-01", 3, now, interval)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NextRefresh("rack-a-bmc-01", 3, now, interval)
	if err != nil || first != second {
		t.Fatalf("refresh not stable: %v %v", first, second)
	}
	if first.Before(now.Add(interval)) || first.After(now.Add(interval+interval/10)) {
		t.Fatalf("refresh %v outside jitter bound", first)
	}
	if _, err := NextRefresh("", 0, now, interval); !errors.Is(err, ErrInvalidRefreshSchedule) {
		t.Fatalf("invalid schedule error=%v", err)
	}
}
