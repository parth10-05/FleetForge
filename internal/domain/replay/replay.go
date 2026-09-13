package replay

import (
	"encoding/json"
	"errors"

	"github.com/parth10-05/fleetforge/internal/domain/campaign"
	"github.com/parth10-05/fleetforge/internal/domain/event"
	"github.com/parth10-05/fleetforge/internal/domain/targetupdate"
)

var ErrUnsupportedVersion = errors.New("unsupported replay version")

// CampaignTrace and TargetUpdateTrace are ordered facts. They do not load an
// event store or invoke a callback; callers are responsible for providing a
// frozen order and retaining the source evidence.
type CampaignTrace struct {
	Version event.Version
	Initial campaign.Aggregate
	Actions []campaign.CommandInput
}

type TargetUpdateTrace struct {
	Version event.Version
	Initial targetupdate.Aggregate
	Actions []targetupdate.CommandInput
}

type CampaignResult struct {
	Version   event.Version          `json:"version"`
	Aggregate CampaignSnapshot       `json:"aggregate"`
	Decisions []CampaignDecisionView `json:"decisions"`
}

type TargetUpdateResult struct {
	Version   event.Version              `json:"version"`
	Aggregate TargetUpdateSnapshot       `json:"aggregate"`
	Decisions []TargetUpdateDecisionView `json:"decisions"`
}

type CampaignSnapshot struct {
	State        string `json:"state"`
	Revision     uint64 `json:"revision"`
	CommandEpoch uint64 `json:"command_epoch"`
	Suspended    string `json:"suspended_phase,omitempty"`
}

type TargetUpdateSnapshot struct {
	State        string `json:"state"`
	Revision     uint64 `json:"revision"`
	CommandEpoch uint64 `json:"command_epoch"`
	ResumeStep   string `json:"resume_step,omitempty"`
}

type CampaignDecisionView struct {
	Class      string   `json:"class"`
	Reason     string   `json:"reason"`
	Events     []string `json:"events,omitempty"`
	NextIntent string   `json:"next_intent,omitempty"`
}

type TargetUpdateDecisionView struct {
	Class        string `json:"class"`
	Reason       string `json:"reason"`
	SubmitIntent bool   `json:"submit_intent"`
	Attention    string `json:"attention,omitempty"`
}

func ReplayCampaign(trace CampaignTrace) (CampaignResult, error) {
	if trace.Version != event.VersionV1 {
		return CampaignResult{}, ErrUnsupportedVersion
	}
	current := trace.Initial
	decisions := make([]CampaignDecisionView, 0, len(trace.Actions))
	for _, action := range trace.Actions {
		decision := campaign.Reduce(current, action)
		current = decision.Aggregate
		decisions = append(decisions, CampaignDecisionView{Class: string(decision.Class), Reason: decision.Reason.String(), Events: append([]string(nil), decision.Events...), NextIntent: decision.NextIntent})
	}
	return CampaignResult{Version: trace.Version, Aggregate: campaignSnapshot(current), Decisions: decisions}, nil
}

func ReplayTargetUpdate(trace TargetUpdateTrace) (TargetUpdateResult, error) {
	if trace.Version != event.VersionV1 {
		return TargetUpdateResult{}, ErrUnsupportedVersion
	}
	current := trace.Initial
	decisions := make([]TargetUpdateDecisionView, 0, len(trace.Actions))
	for _, action := range trace.Actions {
		decision := targetupdate.Reduce(current, action)
		current = decision.Aggregate
		decisions = append(decisions, TargetUpdateDecisionView{Class: string(decision.Class), Reason: decision.Reason.String(), SubmitIntent: decision.SubmitIntent, Attention: string(decision.Attention)})
	}
	return TargetUpdateResult{Version: trace.Version, Aggregate: targetSnapshot(current), Decisions: decisions}, nil
}

// CanonicalJSON emits a byte-stable representation for a replay result. Only
// the two result types are accepted so arbitrary caller maps cannot alter key
// ordering or add unreviewed fields.
func CanonicalJSON(result any) ([]byte, error) {
	switch typed := result.(type) {
	case CampaignResult:
		if typed.Version != event.VersionV1 {
			return nil, ErrUnsupportedVersion
		}
		return json.Marshal(typed)
	case TargetUpdateResult:
		if typed.Version != event.VersionV1 {
			return nil, ErrUnsupportedVersion
		}
		return json.Marshal(typed)
	default:
		return nil, errors.New("unsupported replay result")
	}
}

func campaignSnapshot(aggregate campaign.Aggregate) CampaignSnapshot {
	return CampaignSnapshot{State: aggregate.State.String(), Revision: aggregate.Revision.Uint64(), CommandEpoch: aggregate.CommandEpoch.Uint64(), Suspended: aggregate.SuspendedPhase.String()}
}

func targetSnapshot(aggregate targetupdate.Aggregate) TargetUpdateSnapshot {
	return TargetUpdateSnapshot{State: aggregate.State.String(), Revision: aggregate.Revision.Uint64(), CommandEpoch: aggregate.CommandEpoch.Uint64(), ResumeStep: aggregate.ResumeStep.String()}
}
