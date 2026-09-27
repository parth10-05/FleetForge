package redfishsim

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/simulator/core"
)

func TestCallbackDispatcherDeliversOnlyMatchingSubscription(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		received <- string(body)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	dispatcher := NewCallbackDispatcher(1, 1, time.Second)
	defer dispatcher.Close()
	dispatcher.Enqueue([]core.SubscriptionState{{ID: "one", TargetID: "target-a", Destination: server.URL}, {ID: "two", TargetID: "target-b", Destination: server.URL}}, core.EventState{ID: "event-1", TargetID: "target-a", TaskID: "task-1", Sequence: 1, Kind: "task-running"})
	select {
	case body := <-received:
		if body == "" || dispatcher.Dropped() != 0 {
			t.Fatalf("callback body=%q dropped=%d", body, dispatcher.Dropped())
		}
	case <-time.After(time.Second):
		t.Fatal("callback was not delivered")
	}
}
