package main

import "testing"

func TestSchemaUnavailableErrorIsSafe(t *testing.T) {
	if got := (errP02SchemaUnavailable{}).Error(); got != "P02 schema is unavailable" {
		t.Fatalf("error=%q", got)
	}
}

func TestP02SchemaVersionRequiresLatestP02MigrationButAllowsNewerSchemas(t *testing.T) {
	for _, test := range []struct {
		version int64
		want    bool
	}{
		{version: 0, want: false},
		{version: requiredP02MigrationVersion - 1, want: false},
		{version: requiredP02MigrationVersion, want: true},
		{version: requiredP02MigrationVersion + 1, want: true},
	} {
		if got := p02SchemaVersionReady(test.version); got != test.want {
			t.Fatalf("version %d ready=%v want=%v", test.version, got, test.want)
		}
	}
}
