package controlhttp

import "testing"

func TestRevisionETagRoundTrip(t *testing.T) {
	etag, err := revisionETag(42)
	if err != nil || etag != "\"rev-42\"" {
		t.Fatalf("etag=%q err=%v", etag, err)
	}
	if revision, err := parseRevisionETag(etag); err != nil || revision != 42 {
		t.Fatalf("revision=%d err=%v", revision, err)
	}
	for _, value := range []string{"", "rev-42", "\"rev-0\"", "\"rev-01\"", "\"rev-42\"x"} {
		if _, err := parseRevisionETag(value); err == nil {
			t.Fatalf("accepted invalid ETag %q", value)
		}
	}
}
