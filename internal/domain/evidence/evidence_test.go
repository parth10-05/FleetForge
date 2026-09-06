package evidence

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

const evidenceEventID = "018f2a60-0b71-7dd3-8ae8-2d4f2d2ae9c5"

func TestCategoryAndSourceParsersAreStrict(t *testing.T) {
	for _, category := range AllCategories() {
		got, err := ParseCategory(category.String())
		if err != nil || got != category {
			t.Fatalf("ParseCategory(%q) = %q, %v", category, got, err)
		}
	}
	for _, source := range AllSources() {
		got, err := ParseSource(source.String())
		if err != nil || got != source {
			t.Fatalf("ParseSource(%q) = %q, %v", source, got, err)
		}
	}
	for _, raw := range []string{"", "task", "FUTURE"} {
		_, err := ParseCategory(raw)
		if !errors.Is(err, ErrInvalidCategory) {
			t.Fatalf("ParseCategory(%q) error = %v, want ErrInvalidCategory", raw, err)
		}
		_, err = ParseSource(raw)
		if !errors.Is(err, ErrInvalidSource) {
			t.Fatalf("ParseSource(%q) error = %v, want ErrInvalidSource", raw, err)
		}
	}
}

func TestObservationBoundsFactsAndPreservesMetadata(t *testing.T) {
	metadata := mustMetadata(t, CategoryTask, SourceDevice, 10, 12)
	taskURI, err := NewFact(FactKeyTaskURI, "https://bmc.example/redfish/v1/TaskService/Tasks/42")
	if err != nil {
		t.Fatalf("NewFact(task URI) error = %v", err)
	}
	taskStatus, err := NewFact(FactKeyTaskStatus, "RUNNING")
	if err != nil {
		t.Fatalf("NewFact(task status) error = %v", err)
	}
	observation, err := NewObservation(metadata, []Fact{taskStatus, taskURI})
	if err != nil {
		t.Fatalf("NewObservation() error = %v", err)
	}
	if got, exists := observation.Fact(FactKeyTaskURI); !exists || got != taskURI.Value() {
		t.Fatalf("task URI fact = %q, exists %t", got, exists)
	}
	facts := observation.Facts()
	if len(facts) != 2 || facts[0].Key() != FactKeyTaskURI || facts[1].Key() != FactKeyTaskStatus {
		t.Fatalf("canonical fact order = %#v", facts)
	}
	receivedAt, exists := observation.Metadata().ReceivedAt()
	if !exists || receivedAt.Time().Minute() != 12 {
		t.Fatalf("received time = %v, exists %t", receivedAt, exists)
	}

	_, err = NewObservation(metadata, []Fact{taskURI, taskURI})
	if !errors.Is(err, ErrDuplicateFactKey) {
		t.Fatalf("duplicate fact error = %v, want ErrDuplicateFactKey", err)
	}
	firmwareVersion, err := NewFact(FactKeyFirmwareVersion, "v2")
	if err != nil {
		t.Fatalf("NewFact(firmware version) error = %v", err)
	}
	_, err = NewObservation(metadata, []Fact{firmwareVersion})
	if !errors.Is(err, ErrUnsupportedFact) {
		t.Fatalf("unsupported fact error = %v, want ErrUnsupportedFact", err)
	}
	tooMany := make([]Fact, maxFactsPerObservation+1)
	for index := range tooMany {
		tooMany[index] = taskURI
	}
	_, err = NewObservation(metadata, tooMany)
	if !errors.Is(err, ErrTooManyFacts) {
		t.Fatalf("too many facts error = %v, want ErrTooManyFacts", err)
	}
}

func TestObservationRejectsInvalidMetadataAndRawLikeFactValues(t *testing.T) {
	eventID, err := value.ParseEventID(evidenceEventID)
	if err != nil {
		t.Fatalf("ParseEventID() error = %v", err)
	}
	observedAt, err := value.NewUTCTimestamp(time.Date(2024, time.March, 10, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewUTCTimestamp() error = %v", err)
	}
	receivedBeforeObserved, err := value.NewUTCTimestamp(time.Date(2024, time.March, 10, 9, 59, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewUTCTimestamp() error = %v", err)
	}
	_, err = NewMetadata(eventID, CategoryTask, SourceDevice, observedAt, &receivedBeforeObserved)
	if !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("received-before-observed error = %v, want ErrInvalidMetadata", err)
	}
	_, err = NewMetadata(value.EventID{}, CategoryTask, SourceDevice, observedAt, nil)
	if !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("zero event ID error = %v, want ErrInvalidMetadata", err)
	}

	rawLikeValue := "secret-canary\nraw-body"
	_, err = NewFact(FactKeyTaskURI, rawLikeValue)
	if !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("NewFact(raw-like value) error = %v, want ErrInvalidFact", err)
	}
	if strings.Contains(err.Error(), rawLikeValue) {
		t.Fatalf("fact error leaks raw input: %q", err)
	}
}

func TestObservationIdentityAndStalenessAreExplicit(t *testing.T) {
	first := mustObservation(t, mustMetadata(t, CategoryReachability, SourceDevice, 10, 10))
	second := mustObservation(t, mustMetadata(t, CategoryReachability, SourceReconciliation, 11, 11))
	if !first.SameIdentity(second) {
		t.Fatal("same event ID must identify duplicate evidence")
	}
	if !first.IsOlderThan(second) || second.IsOlderThan(first) {
		t.Fatal("observed timestamps did not classify stale evidence deterministically")
	}
}

func mustMetadata(t *testing.T, category Category, source Source, observedMinute, receivedMinute int) Metadata {
	t.Helper()
	eventID, err := value.ParseEventID(evidenceEventID)
	if err != nil {
		t.Fatalf("ParseEventID() error = %v", err)
	}
	observedAt, err := value.NewUTCTimestamp(time.Date(2024, time.March, 10, 10, observedMinute, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewUTCTimestamp() error = %v", err)
	}
	receivedAt, err := value.NewUTCTimestamp(time.Date(2024, time.March, 10, 10, receivedMinute, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewUTCTimestamp() error = %v", err)
	}
	metadata, err := NewMetadata(eventID, category, source, observedAt, &receivedAt)
	if err != nil {
		t.Fatalf("NewMetadata() error = %v", err)
	}
	return metadata
}

func mustObservation(t *testing.T, metadata Metadata) Observation {
	t.Helper()
	observation, err := NewObservation(metadata, nil)
	if err != nil {
		t.Fatalf("NewObservation() error = %v", err)
	}
	return observation
}
