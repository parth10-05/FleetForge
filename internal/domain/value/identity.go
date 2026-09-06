package value

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxLabelKeyLength   = 63
	maxLabelValueLength = 256
	maxLabelCount       = 32
)

var (
	ErrInvalidLabelKey      = errors.New("invalid label key")
	ErrInvalidLabelValue    = errors.New("invalid label value")
	ErrDuplicateLabelKey    = errors.New("duplicate normalized label key")
	ErrTooManyLabels        = errors.New("too many labels")
	ErrMissingRequiredLabel = errors.New("missing required failure-domain label")
)

// LabelKey is a lowercase ASCII label key.
type LabelKey struct {
	value string
}

func ParseLabelKey(raw string) (LabelKey, error) {
	if len(raw) == 0 || len(raw) > maxLabelKeyLength {
		return LabelKey{}, ErrInvalidLabelKey
	}
	for index := range len(raw) {
		character := raw[index]
		if index == 0 || index == len(raw)-1 {
			if !isASCIIAlphaNumeric(character) {
				return LabelKey{}, ErrInvalidLabelKey
			}
			continue
		}
		if !isASCIIAlphaNumeric(character) && character != '.' && character != '_' && character != '-' {
			return LabelKey{}, ErrInvalidLabelKey
		}
	}
	return LabelKey{value: strings.ToLower(raw)}, nil
}

func (key LabelKey) String() string { return key.value }

func (key LabelKey) IsZero() bool { return key.value == "" }

// LabelValue is a bounded, valid UTF-8 value with no control characters.
type LabelValue struct {
	value string
}

func ParseLabelValue(raw string) (LabelValue, error) {
	if !isBoundedVisibleText(raw, maxLabelValueLength) {
		return LabelValue{}, ErrInvalidLabelValue
	}
	return LabelValue{value: raw}, nil
}

func (value LabelValue) String() string { return value.value }

func (value LabelValue) IsZero() bool { return value.value == "" }

// FailureDomainLabels retains extensible labels while requiring the site and
// rack dimensions used by later rollout policy decisions.
type FailureDomainLabels struct {
	values map[LabelKey]LabelValue
}

func NewFailureDomainLabels(raw map[string]string) (FailureDomainLabels, error) {
	if len(raw) > maxLabelCount {
		return FailureDomainLabels{}, ErrTooManyLabels
	}
	values := make(map[LabelKey]LabelValue, len(raw))
	for rawKey, rawValue := range raw {
		key, err := ParseLabelKey(rawKey)
		if err != nil {
			return FailureDomainLabels{}, err
		}
		value, err := ParseLabelValue(rawValue)
		if err != nil {
			return FailureDomainLabels{}, err
		}
		if _, exists := values[key]; exists {
			return FailureDomainLabels{}, ErrDuplicateLabelKey
		}
		values[key] = value
	}

	siteKey, _ := ParseLabelKey("site")
	rackKey, _ := ParseLabelKey("rack")
	if _, exists := values[siteKey]; !exists {
		return FailureDomainLabels{}, ErrMissingRequiredLabel
	}
	if _, exists := values[rackKey]; !exists {
		return FailureDomainLabels{}, ErrMissingRequiredLabel
	}
	return FailureDomainLabels{values: values}, nil
}

func (labels FailureDomainLabels) Site() LabelValue {
	key, _ := ParseLabelKey("site")
	return labels.values[key]
}

func (labels FailureDomainLabels) Rack() LabelValue {
	key, _ := ParseLabelKey("rack")
	return labels.values[key]
}

func (labels FailureDomainLabels) Get(key LabelKey) (LabelValue, bool) {
	value, exists := labels.values[key]
	return value, exists
}

func (labels FailureDomainLabels) AsMap() map[string]string {
	values := make(map[string]string, len(labels.values))
	for key, value := range labels.values {
		values[key.String()] = value.String()
	}
	return values
}

func isASCIIAlphaNumeric(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'
}

func isBoundedVisibleText(raw string, maximumLength int) bool {
	if len(raw) == 0 || len(raw) > maximumLength || !utf8.ValidString(raw) {
		return false
	}
	for _, character := range raw {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
