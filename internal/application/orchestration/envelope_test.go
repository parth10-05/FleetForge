package orchestration

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testEnvelope() Envelope {
	epoch := int64(7)
	return Envelope{SchemaVersion: WakeupSchemaV1, QueueClass: QueueControl, DueWorkID: "00000000-0000-4000-8000-000000000001", CommandID: "00000000-0000-4000-8000-000000000002", DeliveryGeneration: 0, WorkType: "SAFETY_PROBE", SubjectType: "TARGET_UPDATE", SubjectID: "00000000-0000-4000-8000-000000000003", ExpectedRevision: 1, ExpectedCommandEpoch: &epoch, NotBefore: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), TraceParent: "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"}
}
func TestEnvelopeRoundTrip(t *testing.T) {
	want := testEnvelope()
	raw, err := Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}
func TestDecodeRejectsUnsafeForms(t *testing.T) {
	raw, err := Encode(testEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"unknown": strings.TrimSuffix(string(raw), "}") + `,"secret":"x"}`, "duplicate": strings.TrimSuffix(string(raw), "}") + `,"command_id":"00000000-0000-4000-8000-000000000002"}`, "oversize": `{"schema_version":"` + strings.Repeat("x", MaxEnvelopeBytes) + `"}`} {
		t.Run(name, func(t *testing.T) {
			_, err := Decode([]byte(value))
			if !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
func TestEnvelopeRejectsInvalidFence(t *testing.T) {
	e := testEnvelope()
	e.DeliveryGeneration = -1
	if _, err := Encode(e); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatal(err)
	}
	e = testEnvelope()
	e.NotBefore = e.NotBefore.In(time.FixedZone("offset", 3600))
	if _, err := Encode(e); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatal(err)
	}
}
