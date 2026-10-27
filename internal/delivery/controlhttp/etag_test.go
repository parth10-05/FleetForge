package controlhttp

import (
	"strconv"
	"testing"
)

func TestRevisionETagRoundTrip(t *testing.T) {
	for _, want := range []int64{0, 42} {
		etag, err := revisionETag(want)
		if err != nil || etag != "\"rev-"+strconv.FormatInt(want, 10)+"\"" {
			t.Fatalf("etag=%q err=%v", etag, err)
		}
		if revision, err := parseRevisionETag(etag); err != nil || revision != want {
			t.Fatalf("revision=%d err=%v", revision, err)
		}
	}
	for _, value := range []string{"", "rev-42", "\"rev--1\"", "\"rev-01\"", "\"rev-42\"x"} {
		if _, err := parseRevisionETag(value); err == nil {
			t.Fatalf("accepted invalid ETag %q", value)
		}
	}
}
