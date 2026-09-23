package scenario

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

// Decode accepts exactly one strict JSON document. Unknown and duplicate keys,
// trailing documents, and invalid scenario invariants are rejected.
func Decode(raw []byte) (Scenario, error) {
	if err := rejectDuplicateKeys(raw); err != nil {
		return Scenario{}, invalid("strict JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var scenario Scenario
	if err := decoder.Decode(&scenario); err != nil {
		return Scenario{}, invalid("JSON shape")
	}
	if err := ensureEOF(decoder); err != nil {
		return Scenario{}, invalid("trailing JSON")
	}
	if err := scenario.Validate(); err != nil {
		return Scenario{}, err
	}
	return scenario, nil
}

func CanonicalBytes(s Scenario) ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	// JSON struct fields are emitted in declaration order. List order is part of
	// the scenario contract and is deliberately preserved.
	return json.Marshal(s)
}

func Digest(s Scenario) (string, error) {
	canonical, err := CanonicalBytes(s)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("extra document")
	}
	return nil
}

func rejectDuplicateKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := walkValue(decoder); err != nil {
		return err
	}
	if err := ensureEOF(decoder); err != nil {
		return err
	}
	return nil
}

func walkValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, object := token.(json.Delim)
	if !object {
		return nil
	}
	switch delim {
	case '{':
		keys := map[string]struct{}{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return fmt.Errorf("object key")
			}
			if _, found := keys[name]; found {
				return fmt.Errorf("duplicate key")
			}
			keys[name] = struct{}{}
			if err := walkValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := walkValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected delimiter")
	}
}
