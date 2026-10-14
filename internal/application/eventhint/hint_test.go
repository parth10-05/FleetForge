package eventhint

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func TestParseAcceptsSignedBoundedHint(t *testing.T) {
	secret := []byte("12345678901234567890123456789012")
	body := []byte(`{"Events":[{"EventId":"event-1","Message":"task-running","Oem":{"FleetForge":{"Target":"target-1","Task":"task-1","Sequence":1,"ObservedAt":"2026-10-14T12:00:00Z"}}}]}`)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	hint, err := Parse(secret, hex.EncodeToString(mac.Sum(nil)), body, time.Date(2026, time.October, 14, 12, 0, 1, 0, time.UTC))
	if err != nil || hint.TargetKey != "target-1" || hint.Sequence != 1 || hint.Source != Source {
		t.Fatalf("hint=%+v err=%v", hint, err)
	}
}

func TestParseRejectsUnsignedAndFutureHints(t *testing.T) {
	secret := []byte("12345678901234567890123456789012")
	body := []byte(`{"Events":[{"EventId":"event-1","Message":"task-running","Oem":{"FleetForge":{"Target":"target-1","Task":"task-1","Sequence":1,"ObservedAt":"2026-10-14T12:00:02Z"}}}]}`)
	if _, err := Parse(secret, "", body, time.Date(2026, time.October, 14, 12, 0, 1, 0, time.UTC)); err == nil {
		t.Fatal("unsigned hint was accepted")
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	if _, err := Parse(secret, hex.EncodeToString(mac.Sum(nil)), body, time.Date(2026, time.October, 14, 12, 0, 1, 0, time.UTC)); err == nil {
		t.Fatal("future hint was accepted")
	}
}

func TestParseRejectsDuplicateKeysAndTrailingJSON(t *testing.T) {
	secret := []byte("12345678901234567890123456789012")
	receivedAt := time.Date(2026, time.October, 14, 12, 0, 1, 0, time.UTC)
	for name, body := range map[string][]byte{
		"duplicate key":  []byte(`{"Events":[{"EventId":"event-1","EventId":"event-2","Message":"task-running","Oem":{"FleetForge":{"Target":"target-1","Task":"task-1","Sequence":1,"ObservedAt":"2026-10-14T12:00:00Z"}}}]}`),
		"trailing value": []byte(`{"Events":[{"EventId":"event-1","Message":"task-running","Oem":{"FleetForge":{"Target":"target-1","Task":"task-1","Sequence":1,"ObservedAt":"2026-10-14T12:00:00Z"}}}]} {}`),
	} {
		t.Run(name, func(t *testing.T) {
			mac := hmac.New(sha256.New, secret)
			_, _ = mac.Write(body)
			if _, err := Parse(secret, hex.EncodeToString(mac.Sum(nil)), body, receivedAt); err == nil {
				t.Fatal("ambiguous event document was accepted")
			}
		})
	}
}
