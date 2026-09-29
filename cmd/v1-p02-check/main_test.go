package main

import "testing"

func TestSchemaUnavailableErrorIsSafe(t *testing.T) {
	if got := (errP02SchemaUnavailable{}).Error(); got != "P02 schema is unavailable" {
		t.Fatalf("error=%q", got)
	}
}
