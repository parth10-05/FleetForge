package event

import (
	"encoding/json"
	"strings"
	"testing"
)

func FuzzEventDecoderAndCanonicalizer(f *testing.F) {
	for _, seed := range []string{"{}", `{"version":"v1","name":"campaign.completed"}`, strings.Repeat("x", 2048)} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		var decoded Event
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return
		}
		encoded, err := CanonicalJSON(decoded)
		if err != nil && len(err.Error()) > 128 {
			t.Fatalf("unbounded event error length %d", len(err.Error()))
		}
		if err == nil && len(encoded) > 1024 {
			t.Fatalf("event escaped size bound: %d", len(encoded))
		}
	})
}

func TestEventSchemaCannotCarrySecretOrTransportFields(t *testing.T) {
	typeShape, err := json.Marshal(Event{})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"seed", "secret", "token", "password", "endpoint", "uri", "url", "authorization"} {
		if strings.Contains(strings.ToLower(string(typeShape)), forbidden) {
			t.Fatalf("event schema exposed forbidden field %q", forbidden)
		}
	}
}
