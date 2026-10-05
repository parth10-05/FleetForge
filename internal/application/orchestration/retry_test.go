package orchestration

import (
	"testing"
	"time"
)

func TestRetryAtIsDeterministicAndBounded(t *testing.T) {
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	first, err := RetryAt(now, "00000000-0000-4000-8000-000000000001", 3, time.Second, 8*time.Second, "v1")
	if err != nil {
		t.Fatal(err)
	}
	again, err := RetryAt(now, "00000000-0000-4000-8000-000000000001", 3, time.Second, 8*time.Second, "v1")
	if err != nil || !first.Equal(again) {
		t.Fatalf("first=%s again=%s err=%v", first, again, err)
	}
	if first.Before(now.Add(4*time.Second)) || first.After(now.Add(4400*time.Millisecond)) {
		t.Fatalf("retry %s outside attempt-three bound", first)
	}
	capped, err := RetryAt(now, "00000000-0000-4000-8000-000000000001", 99, time.Second, 8*time.Second, "v1")
	if err != nil || capped.Before(now.Add(8*time.Second)) || capped.After(now.Add(8800*time.Millisecond)) {
		t.Fatalf("capped retry=%s err=%v", capped, err)
	}
}

func TestRetryAtRejectsUnsafeInputs(t *testing.T) {
	if _, err := RetryAt(time.Time{}, "id", 1, time.Second, time.Second, "v1"); err == nil {
		t.Fatal("missing invalid input error")
	}
}
