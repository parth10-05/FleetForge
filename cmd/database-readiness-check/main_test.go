package main

import "testing"

func TestSchemaUnavailableErrorIsSafe(t *testing.T) {
	if got := (errSchemaUnavailable{}).Error(); got != "required schema is unavailable" {
		t.Fatalf("error=%q", got)
	}
}

func TestSchemaVersionRequiresCurrentMigrationButAllowsNewerSchemas(t *testing.T) {
	for _, test := range []struct {
		version int64
		want    bool
	}{
		{version: 0, want: false},
		{version: requiredSchemaVersion - 1, want: false},
		{version: requiredSchemaVersion, want: true},
		{version: requiredSchemaVersion + 1, want: true},
	} {
		if got := p02SchemaVersionReady(test.version); got != test.want {
			t.Fatalf("version %d ready=%v want=%v", test.version, got, test.want)
		}
	}
}
