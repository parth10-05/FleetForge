package inventory

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

const (
	SelectorGrammarV1   = "exact-label-conjunction-v1"
	selectorWireVersion = "fleetforge.selector/v1"
	maxSelectorBytes    = 4 * 1024
	maxSelectorTerms    = 8
)

var ErrInvalidSelector = errors.New("invalid inventory selector")

var selectorKeys = map[string]struct{}{"site": {}, "rack": {}, "group": {}, "family": {}}

// Selector is a canonical bounded conjunction of exact label matches. It has
// no dynamic operators, SQL fragments, paths, or case-folded values.
type Selector struct{ terms []SelectorTerm }
type SelectorTerm struct {
	Key   value.LabelKey
	Value value.LabelValue
}

func ParseSelector(raw []byte) (Selector, error) {
	if len(raw) == 0 || len(raw) > maxSelectorBytes {
		return Selector{}, ErrInvalidSelector
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return Selector{}, ErrInvalidSelector
	}
	var terms []SelectorTerm
	seen := map[string]struct{}{}
	gotVersion := false
	gotAll := false
	for decoder.More() {
		keyRaw, err := decoder.Token()
		if err != nil {
			return Selector{}, ErrInvalidSelector
		}
		key, ok := keyRaw.(string)
		if !ok {
			return Selector{}, ErrInvalidSelector
		}
		if _, duplicate := seen[key]; duplicate {
			return Selector{}, ErrInvalidSelector
		}
		seen[key] = struct{}{}
		switch key {
		case "version":
			var version string
			if decoder.Decode(&version) != nil || version != selectorWireVersion {
				return Selector{}, ErrInvalidSelector
			}
			gotVersion = true
		case "all":
			parsed, err := decodeTerms(decoder)
			if err != nil {
				return Selector{}, ErrInvalidSelector
			}
			terms = parsed
			gotAll = true
		default:
			return Selector{}, ErrInvalidSelector
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || !gotVersion || !gotAll || len(terms) == 0 || decoder.More() {
		return Selector{}, ErrInvalidSelector
	}
	if !errors.Is(decoder.Decode(&struct{}{}), io.EOF) {
		return Selector{}, ErrInvalidSelector
	}
	sort.Slice(terms, func(i, j int) bool { return terms[i].Key.String() < terms[j].Key.String() })
	return Selector{terms: terms}, nil
}

func decodeTerms(decoder *json.Decoder) ([]SelectorTerm, error) {
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		return nil, ErrInvalidSelector
	}
	terms := make([]SelectorTerm, 0, maxSelectorTerms)
	seenKeys := map[string]struct{}{}
	for decoder.More() {
		start, err := decoder.Token()
		if err != nil || start != json.Delim('{') {
			return nil, ErrInvalidSelector
		}
		var key, rawValue string
		seen := map[string]struct{}{}
		for decoder.More() {
			name, err := decoder.Token()
			if err != nil {
				return nil, ErrInvalidSelector
			}
			nameText, ok := name.(string)
			if !ok {
				return nil, ErrInvalidSelector
			}
			if _, duplicate := seen[nameText]; duplicate {
				return nil, ErrInvalidSelector
			}
			seen[nameText] = struct{}{}
			switch nameText {
			case "key":
				if decoder.Decode(&key) != nil {
					return nil, ErrInvalidSelector
				}
			case "value":
				if decoder.Decode(&rawValue) != nil {
					return nil, ErrInvalidSelector
				}
			default:
				return nil, ErrInvalidSelector
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') || len(seen) != 2 {
			return nil, ErrInvalidSelector
		}
		if _, allowed := selectorKeys[key]; !allowed {
			return nil, ErrInvalidSelector
		}
		if _, duplicate := seenKeys[key]; duplicate {
			return nil, ErrInvalidSelector
		}
		seenKeys[key] = struct{}{}
		labelKey, err := value.ParseLabelKey(key)
		if err != nil {
			return nil, ErrInvalidSelector
		}
		labelValue, err := value.ParseLabelValue(rawValue)
		if err != nil {
			return nil, ErrInvalidSelector
		}
		terms = append(terms, SelectorTerm{Key: labelKey, Value: labelValue})
		if len(terms) > maxSelectorTerms {
			return nil, ErrInvalidSelector
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim(']') {
		return nil, ErrInvalidSelector
	}
	return terms, nil
}

func (selector Selector) Terms() []SelectorTerm {
	return append([]SelectorTerm(nil), selector.terms...)
}
func (selector Selector) Matches(labels value.FailureDomainLabels) bool {
	for _, term := range selector.terms {
		actual, ok := labels.Get(term.Key)
		if !ok || actual.String() != term.Value.String() {
			return false
		}
	}
	return true
}
func (selector Selector) CanonicalJSON() []byte {
	result := make([]byte, 0, maxSelectorBytes)
	result = append(result, `{"version":"`...)
	result = append(result, selectorWireVersion...)
	result = append(result, `","all":[`...)
	for index, term := range selector.terms {
		if index > 0 {
			result = append(result, ',')
		}
		key, _ := json.Marshal(term.Key.String())
		val, _ := json.Marshal(term.Value.String())
		result = append(result, `{"key":`...)
		result = append(result, key...)
		result = append(result, `,"value":`...)
		result = append(result, val...)
		result = append(result, '}')
	}
	return append(result, ']', '}')
}
