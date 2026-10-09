package postgres

import (
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/execution"
)

func TestAdmissionRequestFromExecutionPreservesAuthorizedEnvelope(t *testing.T) {
	now := time.Now().UTC()
	command := execution.AdmissionCommand{
		Work:               execution.WorkFence{CommandID: "command", ExpectedRevision: 4},
		Snapshot:           execution.PrecheckSnapshot{CampaignID: "campaign", TargetID: "target", TargetUpdateID: "update", ArtifactID: "artifact", ExpectedVersion: "2.0", CommandEpoch: 7},
		AttemptID:          "attempt",
		StepAttemptID:      "step-attempt",
		AdmissionID:        "admission",
		RequestFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Artifact:           execution.ObjectIdentity{Provider: "local", Container: "firmware", ObjectKey: "verified.bin", ObjectVersion: "v1", SHA256: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 3},
		Envelope: execution.AdmissionEnvelope{
			CampaignTargetID: "campaign-target", ExpectedTargetState: "PRECHECK", NextTargetState: "SUBMITTING", Step: "SUBMISSION",
			Global: execution.CapacityLimit{Level: "GLOBAL", Key: "GLOBAL", Limit: 2}, Site: execution.CapacityLimit{Level: "SITE", Key: "site-a", Limit: 2}, Rack: execution.CapacityLimit{Level: "RACK", Key: "rack-a", Limit: 2}, ReservationIDs: [3]string{"reservation-global", "reservation-site", "reservation-rack"},
			ActorID: "system", CorrelationID: "correlation", CausationID: "causation", Action: "SUBMISSION_ADMITTED", ReasonCode: "target.submission.authorized", OutboxEventID: "outbox", OutboxEventType: "target.submission_admitted", OutboxSchemaVersion: 1, OutboxPayload: []byte(`{"admitted":true}`),
			DueWorkID: "submit-work", DueWorkWorkType: "SUBMIT", DueWorkSchemaVersion: 1, DueWorkNotBefore: now, DecisionInputID: "decision", DecisionLogicalTime: now, ReducerVersion: "domain-v1", DecisionPolicyVersion: "policy-v1", NormalizedFacts: []byte(`{"eligible":true}`), DecisionResult: "SUBMITTING",
		},
	}
	request, err := admissionRequestFromExecution(command)
	if err != nil {
		t.Fatal(err)
	}
	if request.CommandID != command.Work.CommandID || request.ExpectedTargetRevision != command.Work.ExpectedRevision || request.CapturedCommandEpoch != command.Snapshot.CommandEpoch || request.CampaignTargetID != command.Envelope.CampaignTargetID || request.FirmwareAttempt == nil || request.FirmwareAttempt.RequestFingerprint != command.RequestFingerprint || request.FirmwareAttempt.SHA256Digest != command.Artifact.SHA256 {
		t.Fatalf("mapped request lost fenced or immutable identity: %#v", request)
	}
	if request.Global.Key != "GLOBAL" || request.Site.Key != "site-a" || request.Rack.Key != "rack-a" || string(request.OutboxPayload) != `{"admitted":true}` || string(request.NormalizedFacts) != `{"eligible":true}` {
		t.Fatalf("mapped request lost authorized envelope: %#v", request)
	}
}
