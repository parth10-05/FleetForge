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

func TestEnvelopeAcceptsCanonicalPostgreSQLUUIDs(t *testing.T) {
	e := testEnvelope()
	e.DueWorkID = "bddd65f1-78c5-8f75-7634-79c74a4f11b8"
	e.CommandID = "13d3b36b-19ca-df61-ae2b-a82b8e2e8c2f"
	e.SubjectID = "d20a38dd-8e66-1b3c-3de1-6ea99fc9a5a9"
	if _, err := Encode(e); err != nil {
		t.Fatalf("Encode() error = %v, want canonical PostgreSQL UUIDs accepted", err)
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

func TestEnvelopeRejectsInvalidW3CTraceParent(t *testing.T) {
	for name, traceparent := range map[string]string{
		"all-zero-trace-id":  "00-00000000000000000000000000000000-0123456789abcdef-01",
		"all-zero-parent-id": "00-0123456789abcdef0123456789abcdef-0000000000000000-01",
		"reserved-version":   "ff-0123456789abcdef0123456789abcdef-0123456789abcdef-01",
	} {
		t.Run(name, func(t *testing.T) {
			e := testEnvelope()
			e.TraceParent = traceparent
			if _, err := Encode(e); !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("Encode() error = %v, want invalid envelope", err)
			}
		})
	}
}

func TestClassifyEnvelopeFailure(t *testing.T) {
	valid, err := Encode(testEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		raw  []byte
		want string
	}{
		"oversize":        {raw: []byte(strings.Repeat("x", MaxEnvelopeBytes+1)), want: QuarantineOversize},
		"invalid-json":    {raw: []byte("{"), want: QuarantineMalformedEncoding},
		"unknown-version": {raw: []byte(strings.Replace(string(valid), WakeupSchemaV1, "fleetforge.wakeup/v9", 1)), want: QuarantineUnknownVersion},
		"invalid-class":   {raw: []byte(strings.Replace(string(valid), `"control"`, `"other"`, 1)), want: QuarantineInvalidQueueClass},
		"invalid-id":      {raw: []byte(strings.Replace(string(valid), "00000000-0000-4000-8000-000000000001", "not-a-uuid", 1)), want: QuarantineInvalidIdentifier},
		"invalid-fence":   {raw: []byte(strings.Replace(string(valid), `"delivery_generation":0`, `"delivery_generation":-1`, 1)), want: QuarantineInconsistentFencing},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ClassifyEnvelopeFailure(test.raw); got != test.want {
				t.Fatalf("ClassifyEnvelopeFailure() = %q, want %q", got, test.want)
			}
		})
	}
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte(`{"schema_version":"fleetforge.wakeup/v1","queue_class":"control"}`))
	f.Add([]byte(`{"schema_version":"fleetforge.wakeup/v0"}`))
	f.Add([]byte(`not json`))
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = Decode(input)
	})
}
