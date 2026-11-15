package controlhttp

import "testing"

func TestIdempotencyKeyAcceptsOnlyBoundedOpaqueValues(t *testing.T) {
	if got, err := idempotencyKey("request-20261025-0001"); err != nil || got == "" {
		t.Fatalf("valid key=%q err=%v", got, err)
	}
	for _, value := range []string{"", "short", " request-20261025-0001", "request-20261025-0001 ", "request/key/unsafe"} {
		if _, err := idempotencyKey(value); err == nil {
			t.Fatalf("accepted invalid key %q", value)
		}
	}
}
