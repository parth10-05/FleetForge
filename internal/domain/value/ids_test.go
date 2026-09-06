package value

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const validUUID = "018f2a60-0b71-7dd3-8ae8-2d4f2d2ae9c5"

func TestParseCanonicalIDs(t *testing.T) {
	type parseResult struct {
		text string
		zero bool
	}
	testCases := []struct {
		name  string
		parse func(string) (parseResult, error)
	}{
		{name: "campaign", parse: func(raw string) (parseResult, error) {
			id, err := ParseCampaignID(raw)
			return parseResult{id.String(), id.IsZero()}, err
		}},
		{name: "wave", parse: func(raw string) (parseResult, error) {
			id, err := ParseWaveID(raw)
			return parseResult{id.String(), id.IsZero()}, err
		}},
		{name: "target", parse: func(raw string) (parseResult, error) {
			id, err := ParseTargetID(raw)
			return parseResult{id.String(), id.IsZero()}, err
		}},
		{name: "target update", parse: func(raw string) (parseResult, error) {
			id, err := ParseTargetUpdateID(raw)
			return parseResult{id.String(), id.IsZero()}, err
		}},
		{name: "artifact", parse: func(raw string) (parseResult, error) {
			id, err := ParseArtifactID(raw)
			return parseResult{id.String(), id.IsZero()}, err
		}},
		{name: "attempt", parse: func(raw string) (parseResult, error) {
			id, err := ParseAttemptID(raw)
			return parseResult{id.String(), id.IsZero()}, err
		}},
		{name: "command", parse: func(raw string) (parseResult, error) {
			id, err := ParseCommandID(raw)
			return parseResult{id.String(), id.IsZero()}, err
		}},
		{name: "event", parse: func(raw string) (parseResult, error) {
			id, err := ParseEventID(raw)
			return parseResult{id.String(), id.IsZero()}, err
		}},
		{name: "policy version", parse: func(raw string) (parseResult, error) {
			id, err := ParsePolicyVersionID(raw)
			return parseResult{id.String(), id.IsZero()}, err
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := testCase.parse(validUUID)
			if err != nil {
				t.Fatalf("parse canonical UUID: %v", err)
			}
			if result.text != validUUID {
				t.Fatalf("String() = %q, want %q", result.text, validUUID)
			}
			if result.zero {
				t.Fatal("parsed ID is unexpectedly zero")
			}
		})
	}
}

func TestParseCanonicalIDsRejectInvalidInputWithoutEcho(t *testing.T) {
	invalidInputs := []string{
		"",
		"not-a-uuid-secret-canary",
		"018F2A60-0B71-7DD3-8AE8-2D4F2D2AE9C5",
		"018f2a600b717dd38ae82d4f2d2ae9c5",
		"00000000-0000-0000-0000-000000000000",
		validUUID + "x",
	}
	for _, raw := range invalidInputs {
		t.Run("invalid", func(t *testing.T) {
			_, err := ParseCommandID(raw)
			if !errors.Is(err, ErrInvalidCanonicalUUID) {
				t.Fatalf("ParseCommandID() error = %v, want ErrInvalidCanonicalUUID", err)
			}
			if raw != "" && strings.Contains(err.Error(), raw) {
				t.Fatalf("error leaks raw input: %q", err)
			}
		})
	}
}

func TestIDTypesRemainDistinct(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(CampaignID{}),
		reflect.TypeOf(WaveID{}),
		reflect.TypeOf(TargetID{}),
		reflect.TypeOf(TargetUpdateID{}),
		reflect.TypeOf(ArtifactID{}),
		reflect.TypeOf(AttemptID{}),
		reflect.TypeOf(CommandID{}),
		reflect.TypeOf(EventID{}),
		reflect.TypeOf(PolicyVersionID{}),
	}
	for left := range types {
		for right := left + 1; right < len(types); right++ {
			if types[left] == types[right] {
				t.Fatalf("IDs at indexes %d and %d share one Go type", left, right)
			}
		}
	}
}

func TestBoundedCounters(t *testing.T) {
	testCases := []struct {
		name      string
		construct func(int64) (uint64, error)
		valid     int64
		invalid   []int64
		wantError error
	}{
		{name: "revision", construct: func(raw int64) (uint64, error) { value, err := NewRevision(raw); return value.Uint64(), err }, valid: 0, invalid: []int64{-1}, wantError: ErrNegativeRevision},
		{name: "command epoch", construct: func(raw int64) (uint64, error) { value, err := NewCommandEpoch(raw); return value.Uint64(), err }, valid: 0, invalid: []int64{-1}, wantError: ErrNegativeCommandEpoch},
		{name: "attempt number", construct: func(raw int64) (uint64, error) { value, err := NewAttemptNumber(raw); return value.Uint64(), err }, valid: 1, invalid: []int64{0, -1}, wantError: ErrInvalidAttemptNumber},
		{name: "wave ordinal", construct: func(raw int64) (uint64, error) { value, err := NewWaveOrdinal(raw); return value.Uint64(), err }, valid: 1, invalid: []int64{0, -1}, wantError: ErrInvalidWaveOrdinal},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			value, err := testCase.construct(testCase.valid)
			if err != nil {
				t.Fatalf("construct valid value: %v", err)
			}
			if value != uint64(testCase.valid) {
				t.Fatalf("value = %d, want %d", value, testCase.valid)
			}
			for _, invalid := range testCase.invalid {
				_, err := testCase.construct(invalid)
				if !errors.Is(err, testCase.wantError) {
					t.Fatalf("construct(%d) error = %v, want %v", invalid, err, testCase.wantError)
				}
			}
		})
	}
}
