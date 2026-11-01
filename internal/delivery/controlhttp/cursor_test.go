package controlhttp

import (
	"testing"
	"time"
)

func TestTargetCursorRejectsTampering(t *testing.T) {
	key := []byte("abcdefghijklmnopqrstuvwxyz012345")
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	cursor, err := encodeTargetCursor(key, "00000000-0000-0000-0000-000000000001", 25, now)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := decodeTargetCursor(key, cursor, 25, now); err != nil || id != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if _, err := decodeTargetCursor(key, cursor+"x", 25, now); err == nil {
		t.Fatal("accepted tampered cursor")
	}
	if _, err := decodeTargetCursor(key, cursor, 50, now); err == nil {
		t.Fatal("accepted cursor for a different page size")
	}
	if _, err := decodeTargetCursor(key, cursor, 25, now.Add(targetCursorTTL)); err == nil {
		t.Fatal("accepted expired cursor")
	}
}
