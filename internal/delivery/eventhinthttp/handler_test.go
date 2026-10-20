package eventhinthttp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/eventhint"
)

type recordingStore struct {
	hints       []eventhint.Hint
	quarantines []quarantine
}

type quarantine struct {
	body   []byte
	reason string
}

func (store *recordingStore) RecordEventHint(_ context.Context, hint eventhint.Hint) (string, error) {
	store.hints = append(store.hints, hint)
	return "receipt", nil
}

func (store *recordingStore) QuarantineEventHint(_ context.Context, body []byte, reason string, _ time.Time) error {
	store.quarantines = append(store.quarantines, quarantine{body: append([]byte(nil), body...), reason: reason})
	return nil
}

func TestReceiverAcceptsOnlyAuthenticatedBoundedHint(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	secret := []byte("01234567890123456789012345678901")
	store := &recordingStore{}
	body := []byte(`{"Events":[{"EventId":"event-1","EventType":"StatusChange","Message":"TaskCompleted","Oem":{"FleetForge":{"Target":"target-1","Task":"task-1","Sequence":1,"ObservedAt":"2026-01-02T03:04:04Z"}}}]}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/events/synthetic", strings.NewReader(string(body)))
	request.Header.Set(SignatureHeader, signature(secret, body))
	response := httptest.NewRecorder()

	New(secret, store, func() time.Time { return now }).Routes().ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusNoContent)
	}
	if len(store.hints) != 1 || store.hints[0].EventID != "event-1" || store.hints[0].Sequence != 1 || len(store.quarantines) != 0 {
		t.Fatalf("hints=%+v quarantines=%+v", store.hints, store.quarantines)
	}
}

func TestReceiverQuarantinesRejectedSignature(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	store := &recordingStore{}
	body := []byte(`{"Events":[{"EventId":"event-1","Message":"TaskCompleted","Oem":{"FleetForge":{"Target":"target-1","Task":"task-1","Sequence":1,"ObservedAt":"2026-01-02T03:04:04Z"}}}]}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/events/synthetic", strings.NewReader(string(body)))
	request.Header.Set(SignatureHeader, strings.Repeat("0", 64))
	response := httptest.NewRecorder()

	New(secret, store, func() time.Time { return time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC) }).Routes().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want %d", response.Code, http.StatusUnauthorized)
	}
	if len(store.hints) != 0 || len(store.quarantines) != 1 || store.quarantines[0].reason != "REJECTED" || string(store.quarantines[0].body) != string(body) {
		t.Fatalf("hints=%+v quarantines=%+v", store.hints, store.quarantines)
	}
}

func signature(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
