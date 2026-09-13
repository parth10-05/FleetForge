package event

import (
	"encoding/json"
	"errors"

	"github.com/parth10-05/fleetforge/internal/domain/reason"
	"github.com/parth10-05/fleetforge/internal/domain/value"
)

var ErrInvalidEvent = errors.New("invalid domain event")

type Version string

const VersionV1 Version = "v1"

type Name string

const (
	NameCampaignValidationStarted   Name = "campaign.validation_started"
	NameCampaignValidationSucceeded Name = "campaign.validation_succeeded"
	NameCampaignValidationFailed    Name = "campaign.validation_failed"
	NameCampaignCancelled           Name = "campaign.cancelled"
	NameCampaignCanaryStarted       Name = "campaign.canary_started"
	NameCampaignCanaryTerminal      Name = "campaign.canary_terminal"
	NameCampaignCanaryApproved      Name = "campaign.canary_approved"
	NameCampaignWaveReleased        Name = "campaign.wave_released"
	NameCampaignCompleted           Name = "campaign.completed"
	NameCampaignPaused              Name = "campaign.paused"
	NameCampaignResumed             Name = "campaign.resumed"
	NameCampaignAbortStarted        Name = "campaign.abort_started"
	NameCampaignAbortFailed         Name = "campaign.abort_failed"
	NameCampaignAbortCancelled      Name = "campaign.abort_cancelled"
	NameTargetUpdateTransitioned    Name = "target_update.transitioned"
	NameArtifactTransitioned        Name = "artifact.transitioned"
	NameTargetAdministrationChanged Name = "target.administration_changed"
)

var allNames = []Name{
	NameCampaignValidationStarted, NameCampaignValidationSucceeded, NameCampaignValidationFailed,
	NameCampaignCancelled, NameCampaignCanaryStarted, NameCampaignCanaryTerminal,
	NameCampaignCanaryApproved, NameCampaignWaveReleased, NameCampaignCompleted,
	NameCampaignPaused, NameCampaignResumed, NameCampaignAbortStarted,
	NameCampaignAbortFailed, NameCampaignAbortCancelled, NameTargetUpdateTransitioned,
	NameArtifactTransitioned, NameTargetAdministrationChanged,
}

// Event is intentionally a bounded public-decision record. It has no generic
// metadata map, raw device evidence, endpoint, credential, artifact URL, or
// selection seed field, so it is safe to use as the Phase 03 outbox payload.
type Event struct {
	Version     Version     `json:"version"`
	Name        Name        `json:"name"`
	AggregateID string      `json:"aggregate_id"`
	Revision    uint64      `json:"revision"`
	Reason      reason.Code `json:"reason"`
	State       string      `json:"state"`
	NextIntent  string      `json:"next_intent,omitempty"`
}

func New(version Version, name Name, aggregateID string, revision value.Revision, code reason.Code, state, nextIntent string) (Event, error) {
	if version != VersionV1 || !knownName(name) || !canonicalID(aggregateID) || !knownReason(code) || !safeToken(state) || nextIntent != "" && !safeToken(nextIntent) {
		return Event{}, ErrInvalidEvent
	}
	event := Event{Version: version, Name: name, AggregateID: aggregateID, Revision: revision.Uint64(), Reason: code, State: state, NextIntent: nextIntent}
	encoded, err := json.Marshal(event)
	if err != nil || len(encoded) > 1024 {
		return Event{}, ErrInvalidEvent
	}
	return event, nil
}

// CanonicalJSON emits the stable JSON used by fixtures and future transport
// envelopes. Struct fields deliberately fix object-key order.
func CanonicalJSON(event Event) ([]byte, error) {
	if _, err := New(event.Version, event.Name, event.AggregateID, value.Revision(event.Revision), event.Reason, event.State, event.NextIntent); err != nil {
		return nil, err
	}
	return json.Marshal(event)
}

func ParseName(raw string) (Name, error) {
	name := Name(raw)
	if !knownName(name) {
		return "", ErrInvalidEvent
	}
	return name, nil
}

func AllNames() []Name { return append([]Name(nil), allNames...) }

func knownName(name Name) bool {
	for _, candidate := range allNames {
		if candidate == name {
			return true
		}
	}
	return false
}

func canonicalID(raw string) bool       { _, err := value.ParseCampaignID(raw); return err == nil }
func knownReason(code reason.Code) bool { _, err := reason.ParseCode(code.String()); return err == nil }

func safeToken(raw string) bool {
	if len(raw) == 0 || len(raw) > 128 {
		return false
	}
	for _, character := range raw {
		if !(character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' || character == '.' || character == '-') {
			return false
		}
	}
	return true
}
