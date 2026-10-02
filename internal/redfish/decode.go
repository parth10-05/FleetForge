package redfish

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

var (
	// ErrMalformedDocument is returned when a Redfish response cannot be
	// interpreted as one unambiguous JSON document.
	ErrMalformedDocument = errors.New("malformed Redfish document")
	// ErrResponseTooLarge is returned before an unbounded response can be
	// retained or parsed.
	ErrResponseTooLarge = errors.New("Redfish response exceeds size limit")
)

const (
	DefaultDocumentMaxBytes = 1 << 20 // 1 MiB
	DefaultDocumentMaxDepth = 32
)

// DecodeDocument accepts exactly one JSON document after enforcing size and
// nesting bounds. It deliberately rejects duplicate object keys: accepting
// either the first or final spelling would let a device make a capability
// document appear differently to separate consumers.
//
// The returned bytes are the original, bounded wire representation. Callers
// that retain evidence must hash these bytes, never a lossy map decode.
func DecodeDocument(source io.Reader, maxBytes, maxDepth int) (json.RawMessage, error) {
	if source == nil || maxBytes <= 0 || maxDepth <= 0 {
		return nil, ErrMalformedDocument
	}

	limited := io.LimitReader(source, int64(maxBytes)+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, ErrMalformedDocument
	}
	if len(raw) > maxBytes {
		return nil, ErrResponseTooLarge
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := validateJSONValue(decoder, 0, maxDepth); err != nil {
		return nil, ErrMalformedDocument
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, ErrMalformedDocument
	}
	return json.RawMessage(raw), nil
}

func validateJSONValue(decoder *json.Decoder, depth, maxDepth int) error {
	if depth > maxDepth {
		return ErrMalformedDocument
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}

	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			keys := make(map[string]struct{})
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok {
					return ErrMalformedDocument
				}
				if _, exists := keys[name]; exists {
					return ErrMalformedDocument
				}
				keys[name] = struct{}{}
				if err := validateJSONValue(decoder, depth+1, maxDepth); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return ErrMalformedDocument
			}
		case '[':
			for decoder.More() {
				if err := validateJSONValue(decoder, depth+1, maxDepth); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return ErrMalformedDocument
			}
		default:
			return ErrMalformedDocument
		}
	}
	return nil
}
