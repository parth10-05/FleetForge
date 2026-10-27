package controlhttp

import "testing"

func TestTargetCursorRejectsTampering(t *testing.T) {
	key := []byte("abcdefghijklmnopqrstuvwxyz012345")
	cursor, err := encodeTargetCursor(key, "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if id, err := decodeTargetCursor(key, cursor); err != nil || id != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if _, err := decodeTargetCursor(key, cursor+"x"); err == nil {
		t.Fatal("accepted tampered cursor")
	}
}
