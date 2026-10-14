package redfishsim

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/parth10-05/fleetforge/internal/simulator/core"
)

const CallbackSignatureHeader = "X-FleetForge-Event-Signature"

// CallbackDispatcher delivers the intentionally small V1 EventService payload
// using a fixed worker pool and bounded queue. It owns no simulator state;
// callers provide the immutable subscription and event projections.
type CallbackDispatcher struct {
	client *http.Client
	secret []byte
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

func NewCallbackDispatcher(workers, queueSize int, timeout time.Duration, secret []byte) *CallbackDispatcher {
	if workers < 1 {
		workers = 1
	}
	if queueSize < workers {
		queueSize = workers
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	d := &CallbackDispatcher{client: &http.Client{Timeout: timeout}, secret: append([]byte(nil), secret...), queue: make(chan callbackDelivery, queueSize), stop: make(chan struct{}), done: make(chan struct{})}
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
					if len(d.secret) != 0 {
						request.Header.Set(CallbackSignatureHeader, callbackSignature(d.secret, item.body))
					}
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

func callbackSignature(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func (d *CallbackDispatcher) Close()          { d.once.Do(func() { close(d.stop); <-d.done }) }
func (d *CallbackDispatcher) Dropped() uint64 { return d.drops.Load() }

func (d *CallbackDispatcher) Enqueue(subscriptions []core.SubscriptionState, event core.EventState) {
	body, err := json.Marshal(map[string]any{"Events": []map[string]any{{"EventId": event.ID, "EventType": "StatusChange", "Message": event.Kind, "Oem": map[string]any{"FleetForge": map[string]any{"Target": event.TargetID, "Sequence": event.Sequence, "Task": event.TaskID, "ObservedAt": time.Now().UTC().Format(time.RFC3339Nano)}}}}})
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
