// Package eventhint defines the narrow authenticated callback boundary for advisory synthetic Redfish events.
package eventhint

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const Source = "SYNTHETIC_REDFISH_V1"

var ErrRejected = errors.New("event hint rejected")

type Hint struct {
	Source, EventID, TargetKey, TaskID, Kind string
	Sequence                                 int64
	ObservedAt, ReceivedAt                   time.Time
	PayloadDigest                            string
}

// Parse authenticates and bounds one callback before it reaches durable state.
func Parse(secret []byte, signature string, body []byte, receivedAt time.Time) (Hint, error) {
	if len(secret) < 32 || len(body) == 0 || len(body) > 8192 || receivedAt.IsZero() || !validSignature(secret, signature, body) {
		return Hint{}, ErrRejected
	}
	if err := validateUniqueJSON(body); err != nil {
		return Hint{}, ErrRejected
	}
	var envelope struct {
		Events []struct {
			EventID string `json:"EventId"`
			Kind    string `json:"Message"`
			OEM     struct {
				FleetForge struct {
					Target     string `json:"Target"`
					Task       string `json:"Task"`
					Sequence   int64  `json:"Sequence"`
					ObservedAt string `json:"ObservedAt"`
				} `json:"FleetForge"`
			} `json:"Oem"`
		} `json:"Events"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || len(envelope.Events) != 1 {
		return Hint{}, ErrRejected
	}
	event := envelope.Events[0]
	observedAt, err := time.Parse(time.RFC3339Nano, event.OEM.FleetForge.ObservedAt)
	if err != nil || !bounded(event.EventID, 128) || !bounded(event.Kind, 128) || !bounded(event.OEM.FleetForge.Target, 128) || !bounded(event.OEM.FleetForge.Task, 256) || event.OEM.FleetForge.Sequence <= 0 || observedAt.After(receivedAt.UTC()) {
		return Hint{}, ErrRejected
	}
	digest := sha256.Sum256(body)
	return Hint{Source: Source, EventID: event.EventID, TargetKey: event.OEM.FleetForge.Target, TaskID: event.OEM.FleetForge.Task, Kind: event.Kind, Sequence: event.OEM.FleetForge.Sequence, ObservedAt: observedAt.UTC(), ReceivedAt: receivedAt.UTC(), PayloadDigest: "sha256:" + hex.EncodeToString(digest[:])}, nil
}

// validateUniqueJSON accepts one complete JSON value and rejects duplicate
// object members before struct decoding can silently retain only the final one.
func validateUniqueJSON(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := consumeUniqueJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func consumeUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch delimiter, ok := token.(json.Delim); {
	case !ok:
		return nil
	case delimiter == '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, exists := keys[name]; exists {
				return errors.New("duplicate JSON object member")
			}
			keys[name] = struct{}{}
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case delimiter == '[':
		for decoder.More() {
			if err := consumeUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return errors.New("unexpected JSON delimiter")
	}
}

func validSignature(secret []byte, signature string, body []byte) bool {
	decoded, err := hex.DecodeString(signature)
	if err != nil || len(decoded) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return hmac.Equal(decoded, mac.Sum(nil))
}

func bounded(value string, limit int) bool {
	return value != "" && len(value) <= limit && !strings.ContainsAny(value, "\r\n\t")
}
