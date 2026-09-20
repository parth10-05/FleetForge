package postgres

import (
	"context"
	"testing"
	"time"
)

func TestRetentionAndIntegrityRejectUnboundedRequests(t *testing.T) {
	if _, err := CleanupExpiredObservations(context.Background(), nil, time.Time{}, 1); err == nil {
		t.Fatal("expected cleanup validation error")
	}
	if _, err := ScanIntegrity(context.Background(), nil, time.Now(), 0); err == nil {
		t.Fatal("expected scan validation error")
	}
}
