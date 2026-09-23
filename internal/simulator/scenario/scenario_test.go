package scenario

import (
	"errors"
	"strings"
	"testing"
)

func validScenario() Scenario {
	return Scenario{
		SchemaVersion: SchemaVersionV1, ScenarioID: "healthy-six-targets", Seed: "42",
		Topology: Topology{Sites: []string{"iad", "sfo"}},
		Targets: []Target{
			{Index: 5, Site: "iad", Rack: "r2", InitialVersion: "1.0", ExpectedVersion: "1.1", CapabilityProfile: "baseline"},
			{Index: 1, Site: "iad", Rack: "r1", InitialVersion: "1.0", ExpectedVersion: "1.1", CapabilityProfile: "baseline"},
			{Index: 2, Site: "iad", Rack: "r1", InitialVersion: "1.0", ExpectedVersion: "1.1", CapabilityProfile: "baseline"},
			{Index: 3, Site: "sfo", Rack: "r3", InitialVersion: "1.0", ExpectedVersion: "1.1", CapabilityProfile: "baseline"},
			{Index: 4, Site: "sfo", Rack: "r3", InitialVersion: "1.0", ExpectedVersion: "1.1", CapabilityProfile: "baseline"},
			{Index: 6, Site: "sfo", Rack: "r4", InitialVersion: "1.0", ExpectedVersion: "1.1", CapabilityProfile: "baseline"},
		},
		Timing: Timing{UploadMS: 1, TaskStepMS: 2, RebootMS: 3, InventoryVisibilityMS: 4, EventMS: 5},
		Limits: Limits{CallbackWorkers: 1, Subscriptions: 1, BodyBytes: 1024, JournalEntries: 1, OpenConnections: 1, EventsPerSecond: 1},
	}
}

func TestValidateScenarioBoundsAndCrossFields(t *testing.T) {
	if err := validScenario().Validate(); err != nil {
		t.Fatalf("valid scenario: %v", err)
	}
	for name, mutate := range map[string]func(*Scenario){
		"invalid seed": func(s *Scenario) { s.Seed = "not-a-number" },
		"one rack": func(s *Scenario) {
			for i := range s.Targets {
				s.Targets[i].Rack = "r1"
			}
		},
		"unknown target site":    func(s *Scenario) { s.Targets[0].Site = "missing" },
		"duplicate target index": func(s *Scenario) { s.Targets[0].Index = s.Targets[1].Index },
		"deferred fault": func(s *Scenario) {
			s.Faults = []Fault{{FaultID: "f9", Name: "RF-F09", Scope: "all", Trigger: Trigger{Kind: "logical_time"}, Consumption: 1}}
		},
		"unbounded body": func(s *Scenario) { s.Limits.BodyBytes = 16<<20 + 1 },
	} {
		t.Run(name, func(t *testing.T) {
			scenario := validScenario()
			mutate(&scenario)
			if err := scenario.Validate(); !errors.Is(err, ErrInvalidScenario) {
				t.Fatalf("Validate() error = %v, want invalid scenario", err)
			}
		})
	}
}

func TestCanonicalBytesHashAndIdentityAreStable(t *testing.T) {
	scenario := validScenario()
	first, err := CanonicalBytes(scenario)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalBytes(scenario)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || strings.Contains(string(first), "\n") {
		t.Fatalf("canonical bytes are not stable: %q / %q", first, second)
	}
	firstDigest, _ := Digest(scenario)
	secondDigest, _ := Digest(scenario)
	if firstDigest != secondDigest {
		t.Fatal("digest changed for same scenario")
	}
	id, err := ID(scenario, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	changed := scenario
	changed.Seed = "43"
	changedID, err := ID(changed, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	if id == changedID {
		t.Fatal("seed did not namespace identity")
	}
	if err := ValidateIdentityUniqueness([]string{id, id}); !errors.Is(err, ErrInvalidScenario) {
		t.Fatalf("collision error = %v", err)
	}
}
