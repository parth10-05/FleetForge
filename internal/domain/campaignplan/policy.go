package campaignplan

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode"
)

const (
	PolicyVersionV1    = "fleetforge.campaign-policy/v1"
	PlanVersionV1      = "campaign-plan/v1"
	SelectionVersionV1 = "selection/v1"
	maxCandidates      = 10_000
	maxSeedBytes       = 128
)

var ErrInvalidPolicy = errors.New("invalid campaign planning policy")

type SizeUnit string

const (
	SizeUnitAbsolute SizeUnit = "absolute"
	SizeUnitPercent  SizeUnit = "percent"
)

type Size struct {
	Unit  SizeUnit `json:"unit"`
	Value uint64   `json:"value"`
}
type Limits struct {
	Global uint64 `json:"global"`
	Site   uint64 `json:"site"`
	Rack   uint64 `json:"rack"`
}
type Fraction struct {
	Numerator   uint64 `json:"numerator"`
	Denominator uint64 `json:"denominator"`
}
type MaintenanceWindow struct {
	Timezone    string `json:"timezone"`
	Weekdays    []int  `json:"weekdays"`
	StartMinute uint16 `json:"start_minute"`
	EndMinute   uint16 `json:"end_minute"`
}

// Policy is the complete V1 wire contract. Every safety-relevant field is
// mandatory: zero values are never implicit defaults.
type Policy struct {
	Version                 string            `json:"version"`
	Canary                  Size              `json:"canary"`
	Limits                  Limits            `json:"limits"`
	FailureBudget           Fraction          `json:"failure_budget"`
	ExpectedFirmwareVersion string            `json:"expected_firmware_version"`
	RequiredHealthChecks    []string          `json:"required_health_checks"`
	Maintenance             MaintenanceWindow `json:"maintenance"`
	MaximumDurationSeconds  uint64            `json:"maximum_duration_seconds"`
	RetryBudget             uint64            `json:"retry_budget"`
	ObservationWindowSecond uint64            `json:"observation_window_seconds"`
	ObservationEverySecond  uint64            `json:"observation_every_seconds"`
	ObservationFreshSecond  uint64            `json:"observation_freshness_seconds"`
	GovernanceID            string            `json:"governance_id"`
	StrictWorstCaseCapacity bool              `json:"strict_worst_case_capacity"`
}

func (policy Policy) Validate(candidateCount uint64) error {
	if policy.Version != PolicyVersionV1 || candidateCount == 0 || candidateCount > maxCandidates || !validSize(policy.Canary, candidateCount) || !validLimits(policy.Limits, candidateCount) || policy.FailureBudget.Denominator == 0 || policy.FailureBudget.Numerator > policy.FailureBudget.Denominator || !visible(policy.ExpectedFirmwareVersion, 256) || !validChecks(policy.RequiredHealthChecks) || !validMaintenance(policy.Maintenance) || policy.MaximumDurationSeconds == 0 || policy.RetryBudget == 0 || policy.ObservationWindowSecond == 0 || policy.ObservationEverySecond == 0 || policy.ObservationFreshSecond == 0 || policy.ObservationEverySecond > policy.ObservationWindowSecond || policy.StrictWorstCaseCapacity || !visible(policy.GovernanceID, 128) {
		return ErrInvalidPolicy
	}
	canary, err := policy.EffectiveCanary(candidateCount)
	if err != nil || (candidateCount > 1 && canary >= candidateCount) {
		return ErrInvalidPolicy
	}
	return nil
}

func (policy Policy) EffectiveCanary(candidateCount uint64) (uint64, error) {
	if candidateCount == 0 || !validSize(policy.Canary, candidateCount) {
		return 0, ErrInvalidPolicy
	}
	if policy.Canary.Unit == SizeUnitAbsolute {
		return policy.Canary.Value, nil
	}
	return (candidateCount*policy.Canary.Value + 99) / 100, nil
}

// CanonicalJSON normalizes semantically unordered health checks and weekdays;
// the struct field declaration fixes all remaining wire ordering.
func (policy Policy) CanonicalJSON() ([]byte, error) {
	copy := policy
	copy.RequiredHealthChecks = append([]string(nil), policy.RequiredHealthChecks...)
	sort.Strings(copy.RequiredHealthChecks)
	copy.Maintenance.Weekdays = append([]int(nil), policy.Maintenance.Weekdays...)
	sort.Ints(copy.Maintenance.Weekdays)
	encoded, err := json.Marshal(copy)
	if err != nil || bytes.Contains(encoded, []byte("\\u")) {
		return nil, ErrInvalidPolicy
	}
	return encoded, nil
}

func validSize(size Size, count uint64) bool {
	if size.Value == 0 {
		return false
	}
	switch size.Unit {
	case SizeUnitAbsolute:
		return size.Value <= count
	case SizeUnitPercent:
		return size.Value <= 100
	default:
		return false
	}
}
func validLimits(limits Limits, count uint64) bool {
	return limits.Global > 0 && limits.Site > 0 && limits.Rack > 0 && limits.Rack <= limits.Site && limits.Site <= limits.Global && limits.Global <= count
}
func validChecks(checks []string) bool {
	// V1 has one contract-tested post-update health probe. Rejecting unknown
	// names is safer than accepting a frozen policy that the worker cannot
	// collect and then silently marking it healthy.
	if len(checks) != 1 || checks[0] != "boot" {
		return false
	}
	return visible(checks[0], 128)
}
func validMaintenance(window MaintenanceWindow) bool {
	if !visible(window.Timezone, 64) || len(window.Weekdays) == 0 || len(window.Weekdays) > 7 || window.StartMinute >= 1440 || window.EndMinute >= 1440 || window.StartMinute == window.EndMinute {
		return false
	}
	seen := [7]bool{}
	for _, weekday := range window.Weekdays {
		if weekday < 0 || weekday > 6 || seen[weekday] {
			return false
		}
		seen[weekday] = true
	}
	return true
}
func visible(raw string, limit int) bool {
	if raw == "" || len(raw) > limit {
		return false
	}
	for _, rune := range raw {
		if unicode.IsControl(rune) {
			return false
		}
	}
	return !strings.Contains(raw, "\\")
}
