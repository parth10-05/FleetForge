package redfishsim

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/simulator/core"
)

func TestCallbackDispatcherDeliversOnlyMatchingSubscription(t *testing.T) {
	type delivery struct{ body, signature string }
	received := make(chan delivery, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		received <- delivery{body: string(body), signature: request.Header.Get(CallbackSignatureHeader)}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	secret := []byte("12345678901234567890123456789012")
	dispatcher := NewCallbackDispatcher(1, 1, time.Second, secret)
	defer dispatcher.Close()
	dispatcher.Enqueue([]core.SubscriptionState{{ID: "one", TargetID: "target-a", Destination: server.URL}, {ID: "two", TargetID: "target-b", Destination: server.URL}}, core.EventState{ID: "event-1", TargetID: "target-a", TaskID: "task-1", Sequence: 1, Kind: "task-running"})
	select {
	case callback := <-received:
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write([]byte(callback.body))
		if callback.body == "" || callback.signature != hex.EncodeToString(mac.Sum(nil)) || dispatcher.Dropped() != 0 {
			t.Fatalf("callback=%+v dropped=%d", callback, dispatcher.Dropped())
		}
	case <-time.After(time.Second):
		t.Fatal("callback was not delivered")
	}
}
