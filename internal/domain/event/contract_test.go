package event

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/parth10-05/fleetforge/internal/domain/reason"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

func TestCanonicalJSONMatchesVersionedFixture(t *testing.T) {
	revision, _ := value.NewRevision(7)
	event, err := New(VersionV1, NameCampaignWaveReleased, "00000000-0000-4000-8000-000000000001", revision, reason.CodeCampaignWaveAdvanced, "RUNNING_WAVES", "CAMPAIGN.WAVE_RELEASED")
	if err != nil {
		t.Fatal(err)
	}
	got, err := CanonicalJSON(event)
	if err != nil {
		t.Fatal(err)
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	want, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "test", "fixtures", "domain", "v1", "domain-events.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bytes.TrimSpace(want)) {
		t.Fatalf("canonical event = %s, want %s", got, want)
	}
}

func TestEventContractRejectsUnknownNamesAndSecretShapedPayloads(t *testing.T) {
	revision, _ := value.NewRevision(1)
	for _, input := range []struct {
		name   Name
		state  string
		intent string
	}{
		{"future.event", "READY", ""}, {NameCampaignCompleted, "ready", ""}, {NameCampaignCompleted, "READY", "seed=secret"},
	} {
		_, err := New(VersionV1, input.name, "00000000-0000-4000-8000-000000000001", revision, reason.CodeCampaignWaveAdvanced, input.state, input.intent)
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("New(%#v) error = %v", input, err)
		}
	}
}

func TestAllNamesAreStrictAndCopied(t *testing.T) {
	for _, name := range AllNames() {
		if got, err := ParseName(string(name)); err != nil || got != name {
			t.Fatalf("ParseName(%q) = %q, %v", name, got, err)
		}
	}
	names := AllNames()
	names[0] = "changed"
	if AllNames()[0] != NameCampaignValidationStarted {
		t.Fatal("AllNames exposed registry storage")
	}
}
