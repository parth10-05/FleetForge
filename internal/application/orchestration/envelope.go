// Package orchestration defines provider-neutral, non-authoritative work wakeups.
package orchestration

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
)

const (
	WakeupSchemaV1   = "fleetforge.wakeup/v1"
	MaxEnvelopeBytes = 4096

	QuarantineOversize            = "OVERSIZE"
	QuarantineMalformedEncoding   = "MALFORMED_ENCODING"
	QuarantineUnknownVersion      = "UNKNOWN_VERSION"
	QuarantineInvalidIdentifier   = "INVALID_IDENTIFIER"
	QuarantineInvalidQueueClass   = "INVALID_QUEUE_CLASS"
	QuarantineInconsistentFencing = "INCONSISTENT_FENCING"
	QuarantineUnsupportedWorkType = "UNSUPPORTED_WORK_TYPE"
	QuarantinePermanentPublish    = "PERMANENT_PUBLISH_REJECTION"
)

var (
	ErrInvalidEnvelope = errors.New("invalid wakeup envelope")
	uuidPattern        = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	tracePattern       = regexp.MustCompile(`^[\da-f]{2}-[\da-f]{32}-[\da-f]{16}-[\da-f]{2}$`)
)

type QueueClass string

const (
	QueueControl              QueueClass = "control"
	QueueDiscovery            QueueClass = "discovery"
	QueueArtifactVerification QueueClass = "artifact-verification"
	QueueUpdateStep           QueueClass = "update-step"
	QueueReconciliation       QueueClass = "reconciliation"
)

type Envelope struct {
	SchemaVersion        string     `json:"schema_version"`
	QueueClass           QueueClass `json:"queue_class"`
	DueWorkID            string     `json:"due_work_id"`
	CommandID            string     `json:"command_id"`
	DeliveryGeneration   int64      `json:"delivery_generation"`
	WorkType             string     `json:"work_type"`
	SubjectType          string     `json:"subject_type"`
	SubjectID            string     `json:"subject_id"`
	ExpectedState        string     `json:"expected_state,omitempty"`
	ExpectedRevision     int64      `json:"expected_revision"`
	ExpectedCommandEpoch *int64     `json:"expected_command_epoch,omitempty"`
	NotBefore            time.Time  `json:"not_before"`
	TraceParent          string     `json:"traceparent,omitempty"`
}

func (e Envelope) Validate() error {
	if e.SchemaVersion != WakeupSchemaV1 || !validClass(e.QueueClass) || !validUUID(e.DueWorkID) || !validUUID(e.CommandID) || !validUUID(e.SubjectID) || !visible(e.WorkType, 128) || !visible(e.SubjectType, 64) || e.DeliveryGeneration < 0 || e.ExpectedRevision < 0 || e.NotBefore.IsZero() || e.NotBefore.Location() != time.UTC {
		return ErrInvalidEnvelope
	}
	if e.ExpectedCommandEpoch != nil && *e.ExpectedCommandEpoch < 0 {
		return ErrInvalidEnvelope
	}
	if e.ExpectedState != "" && !visible(e.ExpectedState, 64) {
		return ErrInvalidEnvelope
	}
	if e.TraceParent != "" && !validTraceParent(e.TraceParent) {
		return ErrInvalidEnvelope
	}
	return nil
}
func validClass(c QueueClass) bool {
	switch c {
	case QueueControl, QueueDiscovery, QueueArtifactVerification, QueueUpdateStep, QueueReconciliation:
		return true
	}
	return false
}
func validUUID(v string) bool { return uuidPattern.MatchString(v) }

// validTraceParent validates the W3C trace-context fields that matter at this
// boundary. A regex-only check would accept all-zero trace/span IDs, which W3C
// reserves as invalid, and the reserved ff version. The envelope remains
// deliberately opaque beyond this structural validation: tracing is never an
// authority or workflow identity.
func validTraceParent(v string) bool {
	if !tracePattern.MatchString(v) {
		return false
	}
	parts := strings.Split(v, "-")
	return parts[0] != "ff" &&
		parts[1] != strings.Repeat("0", len(parts[1])) &&
		parts[2] != strings.Repeat("0", len(parts[2]))
}
func visible(v string, max int) bool {
	return v != "" && len(v) <= max && !strings.ContainsAny(v, "\\\r\n\t")
}
func Encode(e Envelope) ([]byte, error) {
	if e.Validate() != nil {
		return nil, ErrInvalidEnvelope
	}
	b, err := json.Marshal(e)
	if err != nil || len(b) > MaxEnvelopeBytes {
		return nil, ErrInvalidEnvelope
	}
	return b, nil
}
func Decode(raw []byte) (Envelope, error) {
	if len(raw) == 0 || len(raw) > MaxEnvelopeBytes || hasDuplicateKeys(raw) {
		return Envelope{}, ErrInvalidEnvelope
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var e Envelope
	if d.Decode(&e) != nil || d.Decode(&struct{}{}) != io.EOF || e.Validate() != nil {
		return Envelope{}, ErrInvalidEnvelope
	}
	return e, nil
}

// ClassifyEnvelopeFailure returns a stable, bounded quarantine reason for a
// rejected receipt. It is deliberately diagnostic rather than authoritative:
// the raw receipt is still rejected by Decode and cannot be dispatched.
func ClassifyEnvelopeFailure(raw []byte) string {
	if len(raw) > MaxEnvelopeBytes {
		return QuarantineOversize
	}
	if len(raw) == 0 || hasDuplicateKeys(raw) {
		return QuarantineMalformedEncoding
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return QuarantineMalformedEncoding
	}
	var schemaVersion, queueClass, dueWorkID, commandID, subjectID string
	var generation, revision int64
	if decodeString(fields, "schema_version", &schemaVersion) != nil || schemaVersion != WakeupSchemaV1 {
		return QuarantineUnknownVersion
	}
	if decodeString(fields, "queue_class", &queueClass) != nil || !validClass(QueueClass(queueClass)) {
		return QuarantineInvalidQueueClass
	}
	if decodeString(fields, "due_work_id", &dueWorkID) != nil || decodeString(fields, "command_id", &commandID) != nil || decodeString(fields, "subject_id", &subjectID) != nil || !validUUID(dueWorkID) || !validUUID(commandID) || !validUUID(subjectID) {
		return QuarantineInvalidIdentifier
	}
	if decodeInt64(fields, "delivery_generation", &generation) != nil || decodeInt64(fields, "expected_revision", &revision) != nil || generation < 0 || revision < 0 {
		return QuarantineInconsistentFencing
	}
	return QuarantineMalformedEncoding
}

func decodeString(fields map[string]json.RawMessage, key string, value *string) error {
	raw, ok := fields[key]
	if !ok {
		return ErrInvalidEnvelope
	}
	return json.Unmarshal(raw, value)
}

func decodeInt64(fields map[string]json.RawMessage, key string, value *int64) error {
	raw, ok := fields[key]
	if !ok {
		return ErrInvalidEnvelope
	}
	return json.Unmarshal(raw, value)
}
func hasDuplicateKeys(raw []byte) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func() bool
	walk = func() bool {
		t, err := d.Token()
		if err != nil {
			return true
		}
		x, ok := t.(json.Delim)
		if !ok {
			return false
		}
		if x == '{' {
			seen := map[string]struct{}{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return true
				}
				s, ok := k.(string)
				if !ok {
					return true
				}
				if _, ok = seen[s]; ok {
					return true
				}
				seen[s] = struct{}{}
				if walk() {
					return true
				}
			}
			_, err = d.Token()
			return err != nil
		}
		if x == '[' {
			for d.More() {
				if walk() {
					return true
				}
			}
			_, err = d.Token()
			return err != nil
		}
		return false
	}
	return walk() || d.More()
}
