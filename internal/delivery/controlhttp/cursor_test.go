package controlhttp

import "testing"

func TestTargetCursorRejectsTampering(t *testing.T) {
	key := []byte("abcdefghijklmnopqrstuvwxyz012345")
	cursor, err := encodeTargetCursor(key, "00000000-0000-0000-0000-000000000001", 25)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := decodeTargetCursor(key, cursor, 25); err != nil || id != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if _, err := decodeTargetCursor(key, cursor+"x", 25); err == nil {
		t.Fatal("accepted tampered cursor")
	}
	if _, err := decodeTargetCursor(key, cursor, 50); err == nil {
		t.Fatal("accepted cursor for a different page size")
	}
}
