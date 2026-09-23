// Package scenario defines the strict, route-independent simulator input.
package scenario

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

const SchemaVersionV1 = "fleetforge.simulator.scenario/v1"

const (
	maxTargets = 1000
	maxFaults  = 128
)

var (
	ErrInvalidScenario = errors.New("invalid simulator scenario")
	idPattern          = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

// Scenario is deliberately transport-neutral. It contains no endpoint,
// credential, firmware-byte, or route-specific update data.
type Scenario struct {
	SchemaVersion string   `json:"schema_version"`
	ScenarioID    string   `json:"scenario_id"`
	Seed          string   `json:"seed"`
	Targets       []Target `json:"targets"`
	Topology      Topology `json:"topology"`
	Timing        Timing   `json:"timing"`
	Faults        []Fault  `json:"faults"`
	Limits        Limits   `json:"limits"`
}

type Topology struct {
	Sites []string `json:"sites"`
}

type Target struct {
	Index             int    `json:"index"`
	Site              string `json:"site"`
	Rack              string `json:"rack"`
	InitialVersion    string `json:"initial_version"`
	ExpectedVersion   string `json:"expected_version"`
	CapabilityProfile string `json:"capability_profile"`
}

type Timing struct {
	UploadMS              int `json:"upload_ms"`
	TaskStepMS            int `json:"task_step_ms"`
	RebootMS              int `json:"reboot_ms"`
	InventoryVisibilityMS int `json:"inventory_visibility_ms"`
	EventMS               int `json:"event_ms"`
}

type Limits struct {
	CallbackWorkers int `json:"callback_workers"`
	Subscriptions   int `json:"subscriptions"`
	BodyBytes       int `json:"body_bytes"`
	JournalEntries  int `json:"journal_entries"`
	OpenConnections int `json:"open_connections"`
	EventsPerSecond int `json:"events_per_second"`
}

type Fault struct {
	FaultID     string  `json:"fault_id"`
	Name        string  `json:"name"`
	Scope       string  `json:"scope"`
	ScopeKey    string  `json:"scope_key"`
	Trigger     Trigger `json:"trigger"`
	Consumption int     `json:"consumption"`
	Priority    int     `json:"priority"`
}

type Trigger struct {
	Kind  string `json:"kind"`
	Value int    `json:"value"`
}

func (s Scenario) Validate() error {
	if s.SchemaVersion != SchemaVersionV1 || !validID(s.ScenarioID, 64) {
		return invalid("schema version or scenario identifier")
	}
	if _, err := strconv.ParseUint(s.Seed, 10, 64); err != nil || s.Seed == "" {
		return invalid("seed")
	}
	if len(s.Targets) == 0 || len(s.Targets) > maxTargets || len(s.Faults) > maxFaults {
		return invalid("target or fault count")
	}
	if len(s.Topology.Sites) == 0 || !uniqueIDs(s.Topology.Sites, 64) {
		return invalid("topology sites")
	}
	indices := make(map[int]struct{}, len(s.Targets))
	racks := make(map[string]struct{}, len(s.Targets))
	for _, target := range s.Targets {
		if target.Index < 0 || !validID(target.Site, 64) || !validID(target.Rack, 64) ||
			!boundedText(target.InitialVersion, 256) || !boundedText(target.ExpectedVersion, 256) || !validID(target.CapabilityProfile, 64) {
			return invalid("target")
		}
		if _, found := indices[target.Index]; found {
			return invalid("duplicate target index")
		}
		indices[target.Index] = struct{}{}
		racks[target.Rack] = struct{}{}
		if !contains(s.Topology.Sites, target.Site) {
			return invalid("target site outside topology")
		}
	}
	if len(racks) < 2 {
		return invalid("at least two racks")
	}
	if !validTiming(s.Timing) || !validLimits(s.Limits) {
		return invalid("timing or limits")
	}
	faults := make(map[string]struct{}, len(s.Faults))
	for _, fault := range s.Faults {
		if !validID(fault.FaultID, 64) || !knownFault(fault.Name) || !validScope(fault.Scope, fault.ScopeKey) ||
			!validTrigger(fault.Trigger) || fault.Consumption < 1 || fault.Consumption > 1024 || fault.Priority < 0 || fault.Priority > 1024 {
			return invalid("fault")
		}
		if _, found := faults[fault.FaultID]; found {
			return invalid("duplicate fault identifier")
		}
		faults[fault.FaultID] = struct{}{}
	}
	return nil
}

func invalid(part string) error { return fmt.Errorf("%w: %s", ErrInvalidScenario, part) }

func validID(raw string, max int) bool {
	return len(raw) > 0 && len(raw) <= max && idPattern.MatchString(raw)
}
func boundedText(raw string, max int) bool {
	if len(raw) == 0 || len(raw) > max {
		return false
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func uniqueIDs(values []string, max int) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validID(value, max) {
			return false
		}
		if _, found := seen[value]; found {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}
func validTiming(t Timing) bool {
	return t.UploadMS >= 0 && t.TaskStepMS >= 0 && t.RebootMS >= 0 && t.InventoryVisibilityMS >= 0 && t.EventMS >= 0 && t.UploadMS+t.TaskStepMS+t.RebootMS+t.InventoryVisibilityMS+t.EventMS > 0
}
func validLimits(l Limits) bool {
	return l.CallbackWorkers >= 1 && l.CallbackWorkers <= 64 && l.Subscriptions >= 1 && l.Subscriptions <= 1024 && l.BodyBytes >= 1024 && l.BodyBytes <= 16<<20 && l.JournalEntries >= 1 && l.JournalEntries <= 1_000_000 && l.OpenConnections >= 1 && l.OpenConnections <= 4096 && l.EventsPerSecond >= 1 && l.EventsPerSecond <= 10_000
}
func validScope(scope, key string) bool {
	if scope == "all" {
		return key == ""
	}
	return (scope == "target" || scope == "rack" || scope == "site") && validID(key, 64)
}
func validTrigger(t Trigger) bool {
	return (t.Kind == "logical_time" || t.Kind == "request_count" || t.Kind == "task_state" || t.Kind == "reboot_ordinal") && t.Value >= 0 && t.Value <= 1_000_000
}
func knownFault(name string) bool { _, found := supportedFaults[name]; return found }

var supportedFaults = map[string]struct{}{"RF-F01": {}, "RF-F02": {}, "RF-F03": {}, "RF-F04": {}, "RF-F05": {}, "RF-F06": {}, "RF-F07": {}, "RF-F08": {}, "RF-F10": {}, "RF-F11": {}}
