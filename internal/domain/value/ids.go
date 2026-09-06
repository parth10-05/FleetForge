package value

import "errors"

var (
	// ErrInvalidCanonicalUUID is returned when an identity is empty, nil, or not
	// in lowercase RFC 4122 text form. It intentionally never includes input.
	ErrInvalidCanonicalUUID = errors.New("invalid canonical UUID")
	ErrNegativeRevision     = errors.New("revision must be non-negative")
	ErrNegativeCommandEpoch = errors.New("command epoch must be non-negative")
	ErrInvalidAttemptNumber = errors.New("attempt number must be positive")
	ErrInvalidWaveOrdinal   = errors.New("wave ordinal must be positive")
)

type uuidText string

// CampaignID identifies one campaign aggregate.
type CampaignID struct{ value uuidText }

// WaveID identifies one campaign wave.
type WaveID struct{ value uuidText }

// TargetID identifies an administratively managed target.
type TargetID struct{ value uuidText }

// TargetUpdateID identifies one target-update aggregate.
type TargetUpdateID struct{ value uuidText }

// ArtifactID identifies one firmware artifact aggregate.
type ArtifactID struct{ value uuidText }

// AttemptID identifies one persisted attempt.
type AttemptID struct{ value uuidText }

// CommandID identifies one semantic command.
type CommandID struct{ value uuidText }

// EventID identifies one domain event.
type EventID struct{ value uuidText }

// PolicyVersionID identifies one frozen policy version.
type PolicyVersionID struct{ value uuidText }

func ParseCampaignID(raw string) (CampaignID, error) {
	value, err := parseCanonicalUUID(raw)
	return CampaignID{value: value}, err
}

func ParseWaveID(raw string) (WaveID, error) {
	value, err := parseCanonicalUUID(raw)
	return WaveID{value: value}, err
}

func ParseTargetID(raw string) (TargetID, error) {
	value, err := parseCanonicalUUID(raw)
	return TargetID{value: value}, err
}

func ParseTargetUpdateID(raw string) (TargetUpdateID, error) {
	value, err := parseCanonicalUUID(raw)
	return TargetUpdateID{value: value}, err
}

func ParseArtifactID(raw string) (ArtifactID, error) {
	value, err := parseCanonicalUUID(raw)
	return ArtifactID{value: value}, err
}

func ParseAttemptID(raw string) (AttemptID, error) {
	value, err := parseCanonicalUUID(raw)
	return AttemptID{value: value}, err
}

func ParseCommandID(raw string) (CommandID, error) {
	value, err := parseCanonicalUUID(raw)
	return CommandID{value: value}, err
}

func ParseEventID(raw string) (EventID, error) {
	value, err := parseCanonicalUUID(raw)
	return EventID{value: value}, err
}

func ParsePolicyVersionID(raw string) (PolicyVersionID, error) {
	value, err := parseCanonicalUUID(raw)
	return PolicyVersionID{value: value}, err
}

func (id CampaignID) String() string      { return string(id.value) }
func (id WaveID) String() string          { return string(id.value) }
func (id TargetID) String() string        { return string(id.value) }
func (id TargetUpdateID) String() string  { return string(id.value) }
func (id ArtifactID) String() string      { return string(id.value) }
func (id AttemptID) String() string       { return string(id.value) }
func (id CommandID) String() string       { return string(id.value) }
func (id EventID) String() string         { return string(id.value) }
func (id PolicyVersionID) String() string { return string(id.value) }

func (id CampaignID) IsZero() bool      { return id.value == "" }
func (id WaveID) IsZero() bool          { return id.value == "" }
func (id TargetID) IsZero() bool        { return id.value == "" }
func (id TargetUpdateID) IsZero() bool  { return id.value == "" }
func (id ArtifactID) IsZero() bool      { return id.value == "" }
func (id AttemptID) IsZero() bool       { return id.value == "" }
func (id CommandID) IsZero() bool       { return id.value == "" }
func (id EventID) IsZero() bool         { return id.value == "" }
func (id PolicyVersionID) IsZero() bool { return id.value == "" }

// Revision is a non-negative aggregate revision.
type Revision uint64

func NewRevision(raw int64) (Revision, error) {
	if raw < 0 {
		return 0, ErrNegativeRevision
	}
	return Revision(raw), nil
}

func (value Revision) Uint64() uint64 { return uint64(value) }

// CommandEpoch is a non-negative campaign admission epoch.
type CommandEpoch uint64

func NewCommandEpoch(raw int64) (CommandEpoch, error) {
	if raw < 0 {
		return 0, ErrNegativeCommandEpoch
	}
	return CommandEpoch(raw), nil
}

func (value CommandEpoch) Uint64() uint64 { return uint64(value) }

// AttemptNumber is a one-based attempt sequence number.
type AttemptNumber uint64

func NewAttemptNumber(raw int64) (AttemptNumber, error) {
	if raw <= 0 {
		return 0, ErrInvalidAttemptNumber
	}
	return AttemptNumber(raw), nil
}

func (value AttemptNumber) Uint64() uint64 { return uint64(value) }

// WaveOrdinal is a one-based campaign wave ordinal.
type WaveOrdinal uint64

func NewWaveOrdinal(raw int64) (WaveOrdinal, error) {
	if raw <= 0 {
		return 0, ErrInvalidWaveOrdinal
	}
	return WaveOrdinal(raw), nil
}

func (value WaveOrdinal) Uint64() uint64 { return uint64(value) }

func parseCanonicalUUID(raw string) (uuidText, error) {
	if len(raw) != 36 {
		return "", ErrInvalidCanonicalUUID
	}

	nonZero := false
	for index := range len(raw) {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if raw[index] != '-' {
				return "", ErrInvalidCanonicalUUID
			}
			continue
		}
		if !isLowerHex(raw[index]) {
			return "", ErrInvalidCanonicalUUID
		}
		nonZero = nonZero || raw[index] != '0'
	}
	if !nonZero {
		return "", ErrInvalidCanonicalUUID
	}
	return uuidText(raw), nil
}

func isLowerHex(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f'
}
