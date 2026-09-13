package evidence

import (
	"errors"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

const maxFactsPerObservation = 8

var (
	ErrInvalidCategory         = errors.New("invalid evidence category")
	ErrInvalidSource           = errors.New("invalid evidence source")
	ErrInvalidMetadata         = errors.New("invalid evidence metadata")
	ErrInvalidFact             = errors.New("invalid normalized evidence fact")
	ErrDuplicateFactKey        = errors.New("duplicate normalized evidence fact key")
	ErrUnsupportedFact         = errors.New("unsupported evidence fact for category")
	ErrTooManyFacts            = errors.New("too many normalized evidence facts")
	ErrInvalidAmbiguityOutcome = errors.New("invalid ambiguity outcome")
)

// Category is the normalized class of an observation. It deliberately omits
// raw transport payload categories such as headers or response bodies.
type Category string

const (
	CategoryTask              Category = "TASK"
	CategoryFirmwareInventory Category = "FIRMWARE_INVENTORY"
	CategoryReachability      Category = "REACHABILITY"
	CategoryReboot            Category = "REBOOT"
	CategoryHealth            Category = "HEALTH"
	CategoryRejection         Category = "REJECTION"
	CategoryAmbiguity         Category = "AMBIGUITY"
)

var allCategories = []Category{
	CategoryTask,
	CategoryFirmwareInventory,
	CategoryReachability,
	CategoryReboot,
	CategoryHealth,
	CategoryRejection,
	CategoryAmbiguity,
}

func ParseCategory(raw string) (Category, error) {
	category := Category(raw)
	for _, candidate := range allCategories {
		if category == candidate {
			return category, nil
		}
	}
	return "", ErrInvalidCategory
}

func (category Category) String() string { return string(category) }

func AllCategories() []Category { return append([]Category(nil), allCategories...) }

// Source identifies the normalized source class, not an endpoint, credential,
// header, raw device message, or unbounded vendor error.
type Source string

const (
	SourceDevice         Source = "DEVICE"
	SourceSimulator      Source = "SIMULATOR"
	SourceOperator       Source = "OPERATOR"
	SourceReconciliation Source = "RECONCILIATION"
)

var allSources = []Source{
	SourceDevice,
	SourceSimulator,
	SourceOperator,
	SourceReconciliation,
}

func ParseSource(raw string) (Source, error) {
	source := Source(raw)
	for _, candidate := range allSources {
		if source == candidate {
			return source, nil
		}
	}
	return "", ErrInvalidSource
}

func (source Source) String() string { return string(source) }

func AllSources() []Source { return append([]Source(nil), allSources...) }

// FactKey is an allowlisted normalized fact name. Each observation category
// allows only its own keys.
type FactKey string

const (
	FactKeyTaskURI          FactKey = "task_uri"
	FactKeyTaskStatus       FactKey = "task_status"
	FactKeyFirmwareVersion  FactKey = "firmware_version"
	FactKeyReachable        FactKey = "reachable"
	FactKeyRebootObserved   FactKey = "reboot_observed"
	FactKeyHealthCheck      FactKey = "health_check"
	FactKeyHealthStatus     FactKey = "health_status"
	FactKeyRejectionClass   FactKey = "rejection_class"
	FactKeyAmbiguityOutcome FactKey = "ambiguity_outcome"
	FactKeyAbsenceProof     FactKey = "absence_proof"
)

var allowedFacts = map[Category]map[FactKey]struct{}{
	CategoryTask:              {FactKeyTaskURI: {}, FactKeyTaskStatus: {}},
	CategoryFirmwareInventory: {FactKeyFirmwareVersion: {}},
	CategoryReachability:      {FactKeyReachable: {}},
	CategoryReboot:            {FactKeyRebootObserved: {}},
	CategoryHealth:            {FactKeyHealthCheck: {}, FactKeyHealthStatus: {}},
	CategoryRejection:         {FactKeyRejectionClass: {}},
	CategoryAmbiguity:         {FactKeyAmbiguityOutcome: {}, FactKeyAbsenceProof: {}},
}

// Fact is one bounded normalized observation fact.
type Fact struct {
	key   FactKey
	value value.LabelValue
}

func NewFact(key FactKey, rawValue string) (Fact, error) {
	if !isKnownFactKey(key) {
		return Fact{}, ErrInvalidFact
	}
	boundedValue, err := value.ParseLabelValue(rawValue)
	if err != nil {
		return Fact{}, ErrInvalidFact
	}
	return Fact{key: key, value: boundedValue}, nil
}

func (fact Fact) Key() FactKey { return fact.key }

func (fact Fact) Value() string { return fact.value.String() }

// Metadata is shared, immutable observation context. ReceivedAt is optional
// for sources that do not distinguish device-observed and receipt time.
type Metadata struct {
	id         value.EventID
	category   Category
	source     Source
	observedAt value.UTCTimestamp
	receivedAt *value.UTCTimestamp
}

func NewMetadata(id value.EventID, category Category, source Source, observedAt value.UTCTimestamp, receivedAt *value.UTCTimestamp) (Metadata, error) {
	if id.IsZero() || !isKnownCategory(category) || !isKnownSource(source) || observedAt.IsZero() {
		return Metadata{}, ErrInvalidMetadata
	}
	if receivedAt != nil {
		if receivedAt.IsZero() || receivedAt.Time().Before(observedAt.Time()) {
			return Metadata{}, ErrInvalidMetadata
		}
		copy := *receivedAt
		return Metadata{id: id, category: category, source: source, observedAt: observedAt, receivedAt: &copy}, nil
	}
	return Metadata{id: id, category: category, source: source, observedAt: observedAt}, nil
}

func (metadata Metadata) ID() value.EventID { return metadata.id }

func (metadata Metadata) Category() Category { return metadata.category }

func (metadata Metadata) Source() Source { return metadata.source }

func (metadata Metadata) ObservedAt() value.UTCTimestamp { return metadata.observedAt }

func (metadata Metadata) ReceivedAt() (value.UTCTimestamp, bool) {
	if metadata.receivedAt == nil {
		return value.UTCTimestamp{}, false
	}
	return *metadata.receivedAt, true
}

// Observation is a bounded normalized fact set and does not retain raw device
// payloads, headers, credentials, firmware bytes, or arbitrary vendor text.
type Observation struct {
	metadata Metadata
	facts    map[FactKey]Fact
}

func NewObservation(metadata Metadata, facts []Fact) (Observation, error) {
	if metadata.id.IsZero() || !isKnownCategory(metadata.category) || !isKnownSource(metadata.source) || metadata.observedAt.IsZero() {
		return Observation{}, ErrInvalidMetadata
	}
	if len(facts) > maxFactsPerObservation {
		return Observation{}, ErrTooManyFacts
	}
	normalizedFacts := make(map[FactKey]Fact, len(facts))
	for _, fact := range facts {
		if _, allowed := allowedFacts[metadata.category][fact.key]; !allowed {
			return Observation{}, ErrUnsupportedFact
		}
		if fact.value.IsZero() {
			return Observation{}, ErrInvalidFact
		}
		if _, duplicate := normalizedFacts[fact.key]; duplicate {
			return Observation{}, ErrDuplicateFactKey
		}
		normalizedFacts[fact.key] = fact
	}
	return Observation{metadata: metadata, facts: normalizedFacts}, nil
}

func (observation Observation) Metadata() Metadata { return observation.metadata }

func (observation Observation) Fact(key FactKey) (string, bool) {
	fact, exists := observation.facts[key]
	return fact.Value(), exists
}

func (observation Observation) Facts() []Fact {
	facts := make([]Fact, 0, len(observation.facts))
	for _, key := range allFactKeys() {
		if fact, exists := observation.facts[key]; exists {
			facts = append(facts, fact)
		}
	}
	return facts
}

func (observation Observation) SameIdentity(other Observation) bool {
	return observation.metadata.id == other.metadata.id
}

func (observation Observation) IsOlderThan(other Observation) bool {
	return observation.metadata.observedAt.Time().Before(other.metadata.observedAt.Time())
}

func isKnownCategory(category Category) bool {
	_, err := ParseCategory(category.String())
	return err == nil
}

func isKnownSource(source Source) bool {
	_, err := ParseSource(source.String())
	return err == nil
}

func isKnownFactKey(key FactKey) bool {
	for _, keys := range allowedFacts {
		if _, exists := keys[key]; exists {
			return true
		}
	}
	return false
}

func allFactKeys() []FactKey {
	return []FactKey{
		FactKeyTaskURI,
		FactKeyTaskStatus,
		FactKeyFirmwareVersion,
		FactKeyReachable,
		FactKeyRebootObserved,
		FactKeyHealthCheck,
		FactKeyHealthStatus,
		FactKeyRejectionClass,
		FactKeyAmbiguityOutcome,
		FactKeyAbsenceProof,
	}
}
