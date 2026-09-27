package redfishsim

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/parth10-05/fleetforge/internal/simulator/core"
)

// CallbackDispatcher delivers the intentionally small V1 EventService payload
// using a fixed worker pool and bounded queue. It owns no simulator state;
// callers provide the immutable subscription and event projections.
type CallbackDispatcher struct {
	client *http.Client
	queue  chan callbackDelivery
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
	drops  atomic.Uint64
}

type callbackDelivery struct {
	destination string
	body        []byte
}

func NewCallbackDispatcher(workers, queueSize int, timeout time.Duration) *CallbackDispatcher {
	if workers < 1 {
		workers = 1
	}
	if queueSize < workers {
		queueSize = workers
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	d := &CallbackDispatcher{client: &http.Client{Timeout: timeout}, queue: make(chan callbackDelivery, queueSize), stop: make(chan struct{}), done: make(chan struct{})}
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for {
				select {
				case <-d.stop:
					return
				case item := <-d.queue:
					request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, item.destination, bytes.NewReader(item.body))
					if err != nil {
						continue
					}
					request.Header.Set("Content-Type", "application/json")
					response, err := d.client.Do(request)
					if err == nil {
						_ = response.Body.Close()
					}
				}
			}
		}()
	}
	go func() { group.Wait(); close(d.done) }()
	return d
}

func (d *CallbackDispatcher) Close()          { d.once.Do(func() { close(d.stop); <-d.done }) }
func (d *CallbackDispatcher) Dropped() uint64 { return d.drops.Load() }

func (d *CallbackDispatcher) Enqueue(subscriptions []core.SubscriptionState, event core.EventState) {
	body, err := json.Marshal(map[string]any{"Events": []map[string]any{{"EventId": event.ID, "EventType": "StatusChange", "Message": event.Kind, "Oem": map[string]any{"FleetForge": map[string]any{"Sequence": event.Sequence, "Task": event.TaskID}}}}})
	if err != nil {
		return
	}
	for _, subscription := range subscriptions {
		if subscription.TargetID != event.TargetID || subscription.Destination == "" {
			continue
		}
		select {
		case d.queue <- callbackDelivery{destination: subscription.Destination, body: body}:
		default:
			d.drops.Add(1)
		}
	}
}
