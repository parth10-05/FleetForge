// Package redfishsim owns the public synthetic Redfish-shaped listener.
package redfishsim

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/parth10-05/fleetforge/internal/simulator/core"
	"github.com/parth10-05/fleetforge/internal/simulator/journal"
)

// Handler exposes only the bounded V1 target surface. It has no simulator
// administration route; that surface remains on its own listener.
type Handler struct {
	engine         *core.Engine
	user, password []byte
	timeout        time.Duration
	maxBody        int64
	journal        *journal.Writer
	journalMu      sync.Mutex
	afterMutation  func(context.Context) error
	dispatcher     *CallbackDispatcher
	callbackMu     sync.Mutex
	delayedEvent   *core.EventState
	connections    chan struct{}
	metrics        counters
}

type counters struct{ requests, acceptedUpdates, taskObservations, faults atomic.Uint64 }
type Metrics struct {
	Requests         uint64 `json:"requests"`
	AcceptedUpdates  uint64 `json:"accepted_updates"`
	TaskObservations uint64 `json:"task_observations"`
	Faults           uint64 `json:"faults"`
}

// NewWithJournal composes a retained run journal with the target listener.
// The writer synchronizes request receipt and acceptance evidence before a
// success response can leave the process.
func NewWithJournal(engine *core.Engine, user, password []byte, timeout time.Duration, maxBody int64, writer *journal.Writer) *Handler {
	h := New(engine, user, password, timeout, maxBody)
	h.journal = writer
	return h
}

func New(engine *core.Engine, user, password []byte, timeout time.Duration, maxBody int64) *Handler {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if maxBody < 1024 {
		maxBody = 1024
	}
	return &Handler{engine: engine, user: append([]byte(nil), user...), password: append([]byte(nil), password...), timeout: timeout, maxBody: maxBody}
}

// SetAfterMutation installs the retained-state checkpoint callback. The
// callback is invoked before a successful mutation response is written, so a
// graceful restart never acknowledges state that it cannot restore.
func (h *Handler) SetAfterMutation(callback func(context.Context) error) { h.afterMutation = callback }
func (h *Handler) SetCallbackDispatcher(dispatcher *CallbackDispatcher)  { h.dispatcher = dispatcher }
func (h *Handler) SetOpenConnectionLimit(limit int) {
	if limit < 1 {
		limit = 1
	}
	h.connections = make(chan struct{}, limit)
}

// Routes preserves the non-composed isolation boundary used by existing tests.
func Routes() http.Handler { return http.MaxBytesHandler(http.NewServeMux(), 4<<10) }
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/virtual-bmcs/", h.handle)
	return h.limitConnections(http.MaxBytesHandler(mux, h.maxBody))
}
func (h *Handler) limitConnections(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if h.connections == nil {
			next.ServeHTTP(writer, request)
			return
		}
		select {
		case h.connections <- struct{}{}:
			defer func() { <-h.connections }()
			next.ServeHTTP(writer, request)
		default:
			errorJSON(writer, http.StatusServiceUnavailable, "Base.1.0.ServiceTemporarilyUnavailable", "Synthetic listener connection limit is reached.")
		}
	})
}
func (h *Handler) Metrics() Metrics {
	return Metrics{Requests: h.metrics.requests.Load(), AcceptedUpdates: h.metrics.acceptedUpdates.Load(), TaskObservations: h.metrics.taskObservations.Load(), Faults: h.metrics.faults.Load()}
}

// DispatchEvents queues callbacks for lifecycle events fired by an external
// logical-time adapter. The engine remains transport-neutral and serial.
func (h *Handler) DispatchEvents(ctx context.Context, result core.Result) {
	if h.dispatcher == nil || len(result.Fired) == 0 {
		return
	}
	for _, fired := range result.Fired {
		parts := strings.SplitN(fired.Value, ":", 2)
		if len(parts) != 2 {
			continue
		}
		for _, event := range result.Events {
			if event.TargetID == fired.TargetID && event.TaskID == parts[1] && event.Kind == parts[0] {
				h.dispatchEvent(ctx, result, event)
				break
			}
		}
	}
}

func (h *Handler) dispatchEvent(ctx context.Context, result core.Result, event core.EventState) {
	if h.consumeEventFault(ctx, result, event, "RF-F03") {
		return
	}
	if h.consumeEventFault(ctx, result, event, "RF-F02") {
		h.callbackMu.Lock()
		copy := event
		h.delayedEvent = &copy
		h.callbackMu.Unlock()
		return
	}
	h.callbackMu.Lock()
	delayed := h.delayedEvent
	h.delayedEvent = nil
	h.callbackMu.Unlock()
	h.dispatcher.Enqueue(result.Subscriptions, event)
	if delayed != nil {
		h.dispatcher.Enqueue(result.Subscriptions, *delayed)
	}
	if h.consumeEventFault(ctx, result, event, "RF-F01") {
		h.dispatcher.Enqueue(result.Subscriptions, event)
	}
}

func (h *Handler) consumeEventFault(ctx context.Context, result core.Result, event core.EventState, name string) bool {
	state := targetState(result, event.TargetID)
	for _, fault := range result.Faults {
		if fault.Name != name || fault.Remaining < 1 || !matchesFault(fault, state) || !triggered(fault, result, state) {
			continue
		}
		if _, err := h.engine.Submit(ctx, core.ConsumeFault{TargetID: event.TargetID, FaultID: fault.ID}); err != nil {
			return false
		}
		if h.checkpoint(ctx) != nil {
			return false
		}
		h.metrics.faults.Add(1)
		return true
	}
	return false
}

func (h *Handler) handle(w http.ResponseWriter, r *http.Request) {
	h.metrics.requests.Add(1)
	if !h.auth(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="fleetforge-simulator"`)
		errorJSON(w, http.StatusUnauthorized, "Base.1.0.NoValidSession", "Target authentication is required.")
		return
	}
	target, resource, ok := path(r.URL.Path)
	if !ok {
		errorJSON(w, http.StatusNotFound, "Base.1.0.ResourceMissingAtURI", "The requested synthetic resource does not exist.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	result, err := h.engine.Submit(ctx, core.Observe{TargetID: target})
	if err != nil {
		errorJSON(w, http.StatusNotFound, "Base.1.0.ResourceMissingAtURI", "The requested synthetic resource does not exist.")
		return
	}
	if r.Method == http.MethodPost && resource == "UpdateService/update" {
		h.update(w, r, ctx, target)
		return
	}
	if r.Method == http.MethodPost && resource == "EventService/Subscriptions" {
		h.subscribe(w, r, ctx, target)
		return
	}
	if r.Method != http.MethodGet {
		errorJSON(w, http.StatusMethodNotAllowed, "Base.1.0.ActionNotSupported", "The requested synthetic method is not supported.")
		return
	}
	if h.consumeObservationFault(w, ctx, target, result) {
		return
	}
	prefix := "/virtual-bmcs/" + target + "/redfish/v1"
	switch resource {
	case "":
		jsonOut(w, http.StatusOK, map[string]any{"@odata.type": "#ServiceRoot.v1_9_4.ServiceRoot", "Id": "RootService", "RedfishVersion": "1.16.0", "Systems": link(prefix + "/Systems"), "Managers": link(prefix + "/Managers"), "Chassis": link(prefix + "/Chassis"), "UpdateService": link(prefix + "/UpdateService"), "TaskService": link(prefix + "/TaskService"), "EventService": link(prefix + "/EventService"), "Oem": map[string]any{"FleetForge": map[string]bool{"Synthetic": true}}})
	case "Systems":
		jsonOut(w, http.StatusOK, map[string]any{"Members": []any{link(prefix + "/Systems/1")}, "Members@odata.count": 1})
	case "Systems/1", "Managers/1", "Chassis/1":
		jsonOut(w, http.StatusOK, identity(resource, targetState(result, target)))
	case "Managers", "Chassis":
		jsonOut(w, http.StatusOK, map[string]any{"Members": []any{link(prefix + "/" + resource + "/1")}, "Members@odata.count": 1})
	case "UpdateService":
		jsonOut(w, http.StatusOK, map[string]any{"@odata.type": "#UpdateService.v1_9_8.UpdateService", "Id": "UpdateService", "ServiceEnabled": true, "FirmwareInventory": link(prefix + "/UpdateService/FirmwareInventory"), "HttpPushUri": prefix + "/UpdateService/update", "Oem": map[string]any{"FleetForge": map[string]any{"UpdateHeaders": []string{"X-FleetForge-Expected-Version", "X-FleetForge-Artifact-SHA256", "X-FleetForge-Attempt"}}}})
	case "TaskService/Tasks":
		tasks := []any{}
		for _, v := range result.Tasks {
			if v.TargetID == target && v.Visible {
				tasks = append(tasks, link(prefix+"/TaskService/Tasks/"+v.ID))
			}
		}
		jsonOut(w, http.StatusOK, map[string]any{"Members": tasks, "Members@odata.count": len(tasks)})
	case "UpdateService/FirmwareInventory":
		jsonOut(w, http.StatusOK, map[string]any{"Members": []any{link(prefix + "/UpdateService/FirmwareInventory/BMC")}, "Members@odata.count": 1})
	case "UpdateService/FirmwareInventory/BMC":
		jsonOut(w, http.StatusOK, inventory(targetState(result, target)))
	case "TaskService":
		jsonOut(w, http.StatusOK, map[string]any{"@odata.type": "#TaskService.v1_3_0.TaskService", "Id": "TaskService", "ServiceEnabled": true, "Tasks": link(prefix + "/TaskService/Tasks")})
	case "EventService":
		jsonOut(w, http.StatusOK, map[string]any{"@odata.type": "#EventService.v1_9_4.EventService", "Id": "EventService", "ServiceEnabled": true, "Subscriptions": link(prefix + "/EventService/Subscriptions"), "Oem": map[string]any{"FleetForge": map[string]string{"EventSequence": "logical"}}})
	case "EventService/Subscriptions":
		members := make([]any, 0, len(result.Subscriptions))
		for _, subscription := range result.Subscriptions {
			if subscription.TargetID == target {
				members = append(members, link(prefix+"/EventService/Subscriptions/"+subscription.ID))
			}
		}
		jsonOut(w, http.StatusOK, map[string]any{"Members": members, "Members@odata.count": len(members)})
	default:
		if strings.HasPrefix(resource, "TaskService/Tasks/") {
			h.task(w, strings.TrimPrefix(resource, "TaskService/Tasks/"), result)
			return
		}
		errorJSON(w, http.StatusNotFound, "Base.1.0.ResourceMissingAtURI", "The requested synthetic resource does not exist.")
	}
}

func (h *Handler) subscribe(w http.ResponseWriter, r *http.Request, ctx context.Context, target string) {
	body := http.MaxBytesReader(w, r.Body, h.maxBody)
	defer body.Close()
	raw, err := io.ReadAll(body)
	if err != nil {
		errorJSON(w, http.StatusRequestEntityTooLarge, "Base.1.0.RequestTooLarge", "The subscription body is too large.")
		return
	}
	var request subscriptionRequest
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || !validSubscription(request) {
		errorJSON(w, http.StatusBadRequest, "Base.1.0.PropertyValueIncorrect", "Synthetic subscription is invalid.")
		return
	}
	hash := sha256.Sum256(raw)
	result, err := h.engine.Submit(ctx, core.CreateSubscription{TargetID: target, Fingerprint: hex.EncodeToString(hash[:]), Destination: request.Destination})
	if err != nil || len(result.Subscriptions) == 0 {
		errorJSON(w, http.StatusBadRequest, "Base.1.0.PropertyValueIncorrect", "Synthetic subscription is invalid.")
		return
	}
	if err := h.checkpoint(ctx); err != nil {
		errorJSON(w, http.StatusServiceUnavailable, "Base.1.0.InternalError", "Synthetic retained state is unavailable.")
		return
	}
	id := result.Subscriptions[len(result.Subscriptions)-1].ID
	uri := "/virtual-bmcs/" + target + "/redfish/v1/EventService/Subscriptions/" + id
	w.Header().Set("Location", uri)
	jsonOut(w, http.StatusCreated, map[string]any{"@odata.type": "#EventDestination.v1_9_9.EventDestination", "Id": id, "Destination": request.Destination, "Protocol": request.Protocol, "SubscriptionType": request.SubscriptionType})
}

type subscriptionRequest struct {
	Destination      string `json:"Destination"`
	Protocol         string `json:"Protocol"`
	SubscriptionType string `json:"SubscriptionType"`
}

func validSubscription(request subscriptionRequest) bool {
	endpoint, err := url.ParseRequestURI(request.Destination)
	return err == nil && (endpoint.Scheme == "http" || endpoint.Scheme == "https") && endpoint.Host != "" && request.Protocol == "Redfish" && request.SubscriptionType == "RedfishEvent"
}

func (h *Handler) task(w http.ResponseWriter, id string, result core.Result) {
	for _, v := range result.Tasks {
		if v.ID == id && v.Visible {
			h.metrics.taskObservations.Add(1)
			jsonOut(w, http.StatusOK, map[string]any{"@odata.type": "#Task.v1_7_4.Task", "Id": v.ID, "TaskState": string(v.State), "TaskStatus": "OK", "PercentComplete": v.PercentComplete, "Messages": []any{}, "Oem": map[string]any{"FleetForge": map[string]string{"OperationFingerprint": "synthetic"}}})
			return
		}
	}
	errorJSON(w, http.StatusNotFound, "Base.1.0.ResourceMissingAtURI", "The requested synthetic resource does not exist.")
}
func targetState(result core.Result, id string) core.TargetState {
	for _, v := range result.State {
		if v.ID == id {
			return v
		}
	}
	return core.TargetState{}
}
func identity(resource string, state core.TargetState) map[string]any {
	kind := "ComputerSystem"
	if strings.HasPrefix(resource, "Managers") {
		kind = "Manager"
	}
	if strings.HasPrefix(resource, "Chassis") {
		kind = "Chassis"
	}
	health := "OK"
	if !state.Healthy {
		health = "Critical"
	}
	return map[string]any{"@odata.type": "#" + kind + ".v1_9_9." + kind, "Id": "1", "Name": "Synthetic " + kind, "Status": map[string]string{"State": "Enabled", "Health": health}, "Oem": map[string]any{"FleetForge": map[string]string{"Model": "deterministic"}}}
}
func inventory(state core.TargetState) map[string]any {
	health := "OK"
	if !state.Healthy {
		health = "Critical"
	}
	return map[string]any{"@odata.type": "#SoftwareInventory.v1_9_2.SoftwareInventory", "Id": "BMC", "Version": state.Version, "Updateable": true, "Status": map[string]string{"State": "Enabled", "Health": health}, "Oem": map[string]any{"FleetForge": map[string]string{"ExpectedVersion": state.StagedVersion}}}
}

func (h *Handler) consumeObservationFault(w http.ResponseWriter, ctx context.Context, target string, result core.Result) bool {
	state := core.TargetState{}
	for _, candidate := range result.State {
		if candidate.ID == target {
			state = candidate
			break
		}
	}
	for _, fault := range result.Faults {
		if fault.Remaining < 1 || (fault.Name != "RF-F04" && fault.Name != "RF-F05") {
			continue
		}
		matches := fault.Scope == "all" || (fault.Scope == "target" && fault.ScopeKey == target) || (fault.Scope == "rack" && fault.ScopeKey == state.Rack) || (fault.Scope == "site" && fault.ScopeKey == state.Site)
		if !matches {
			continue
		}
		if _, err := h.engine.Submit(ctx, core.ConsumeFault{TargetID: target, FaultID: fault.ID}); err != nil {
			return false
		}
		if h.checkpoint(ctx) != nil {
			errorJSON(w, http.StatusServiceUnavailable, "Base.1.0.InternalError", "Synthetic retained state is unavailable.")
			return true
		}
		h.metrics.faults.Add(1)
		if fault.Name == "RF-F04" {
			w.Header().Set("Retry-After", "1")
			errorJSON(w, http.StatusTooManyRequests, "Base.1.0.ServiceTemporarilyUnavailable", "Synthetic observation is rate limited.")
		} else {
			errorJSON(w, http.StatusServiceUnavailable, "Base.1.0.ServiceTemporarilyUnavailable", "Synthetic observation is unavailable.")
		}
		return true
	}
	return false
}

func (h *Handler) auth(r *http.Request) bool {
	user, password, ok := r.BasicAuth()
	return ok && len(user) == len(h.user) && len(password) == len(h.password) && len(h.user) > 0 && subtle.ConstantTimeCompare([]byte(user), h.user) == 1 && subtle.ConstantTimeCompare([]byte(password), h.password) == 1
}
func path(raw string) (string, string, bool) {
	const p = "/virtual-bmcs/"
	if !strings.HasPrefix(raw, p) {
		return "", "", false
	}
	a := strings.Split(strings.TrimPrefix(raw, p), "/")
	if len(a) < 3 || a[0] == "" || a[1] != "redfish" || a[2] != "v1" {
		return "", "", false
	}
	return a[0], strings.Join(a[3:], "/"), true
}
func (h *Handler) update(w http.ResponseWriter, r *http.Request, ctx context.Context, target string) {
	if !strings.EqualFold(r.Header.Get("Content-Type"), "application/octet-stream") {
		errorJSON(w, http.StatusUnsupportedMediaType, "Base.1.0.MediaTypeError", "Synthetic updates require application/octet-stream.")
		return
	}
	version, digest, attempt := r.Header.Get("X-FleetForge-Expected-Version"), r.Header.Get("X-FleetForge-Artifact-SHA256"), r.Header.Get("X-FleetForge-Attempt")
	if version == "" || attempt == "" || len(digest) != 64 {
		errorJSON(w, http.StatusBadRequest, "Base.1.0.PropertyValueFormatError", "Synthetic update headers are invalid.")
		return
	}
	if h.consumeTransportFault(ctx, target, "RF-F06") {
		// A pre-write transport fault deliberately leaves no Redfish response.
		// On a real listener the client observes a closed connection before its
		// body can be accepted.
		if h.checkpoint(ctx) == nil {
			h.abort(w)
		}
		return
	}
	body := http.MaxBytesReader(w, r.Body, h.maxBody)
	defer body.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, body); err != nil {
		errorJSON(w, http.StatusRequestEntityTooLarge, "Base.1.0.RequestTooLarge", "The synthetic update body is too large.")
		return
	}
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(digest)), []byte(hex.EncodeToString(hash.Sum(nil)))) != 1 {
		errorJSON(w, http.StatusBadRequest, "Base.1.0.PropertyValueIncorrect", "Synthetic update digest does not match the body.")
		return
	}
	h.journalMu.Lock()
	defer h.journalMu.Unlock()
	if h.journal != nil {
		if _, err := h.journal.Append(journal.Record{Kind: journal.RequestReceived, TargetID: target, Payload: journal.Payload{CorrelationID: attempt, Fingerprint: strings.ToLower(digest)}}); err != nil {
			errorJSON(w, http.StatusServiceUnavailable, "Base.1.0.InternalError", "Synthetic request evidence is unavailable.")
			return
		}
	}
	result, err := h.engine.Submit(ctx, core.AcceptUpdate{TargetID: target, Version: version, AttemptID: attempt})
	if err != nil || len(result.Operations) == 0 {
		errorJSON(w, http.StatusBadRequest, "Base.1.0.PropertyValueIncorrect", "Synthetic update does not match target policy.")
		return
	}
	op := result.Operations[len(result.Operations)-1]
	if h.journal != nil {
		if _, err := h.journal.Append(journal.Record{LogicalTime: result.Time, Kind: journal.UpdateAccepted, TargetID: target, Payload: journal.Payload{CorrelationID: attempt, Fingerprint: strings.ToLower(digest)}}); err != nil {
			errorJSON(w, http.StatusServiceUnavailable, "Base.1.0.InternalError", "Synthetic acceptance evidence is unavailable.")
			return
		}
	}
	if err := h.checkpoint(ctx); err != nil {
		errorJSON(w, http.StatusServiceUnavailable, "Base.1.0.InternalError", "Synthetic retained state is unavailable.")
		return
	}
	h.metrics.acceptedUpdates.Add(1)
	if h.consumeTransportFault(ctx, target, "RF-F07") {
		if err := h.checkpoint(ctx); err != nil {
			return
		}
		// Acceptance and its checkpoint are durable before the deliberate loss.
		h.abort(w)
		return
	}
	uri := "/virtual-bmcs/" + target + "/redfish/v1/TaskService/Tasks/" + op.TaskID
	w.Header().Set("Location", uri)
	jsonOut(w, http.StatusAccepted, map[string]any{"@odata.id": uri, "Oem": map[string]any{"FleetForge": map[string]string{"Attempt": attempt}}})
}

func (h *Handler) consumeTransportFault(ctx context.Context, target, name string) bool {
	result, err := h.engine.Submit(ctx, core.Observe{TargetID: target})
	if err != nil {
		return false
	}
	state := targetState(result, target)
	for _, fault := range result.Faults {
		if fault.Name != name || fault.Remaining < 1 || !matchesFault(fault, state) || !triggered(fault, result, state) {
			continue
		}
		if _, err := h.engine.Submit(ctx, core.ConsumeFault{TargetID: target, FaultID: fault.ID}); err == nil {
			h.metrics.faults.Add(1)
			return true
		}
	}
	return false
}

func matchesFault(fault core.FaultState, state core.TargetState) bool {
	return fault.Scope == "all" || (fault.Scope == "target" && fault.ScopeKey == state.ID) || (fault.Scope == "rack" && fault.ScopeKey == state.Rack) || (fault.Scope == "site" && fault.ScopeKey == state.Site)
}

// Trigger values are evaluated against the immutable observation projection.
// task_state and reboot_ordinal are both acceptance ordinals in the small v1
// profile; later phases may introduce named task-state predicates.
func triggered(fault core.FaultState, result core.Result, state core.TargetState) bool {
	switch fault.TriggerKind {
	case "logical_time":
		return int64(result.Time) >= int64(fault.TriggerValue)
	case "request_count":
		return state.RequestCount >= uint64(fault.TriggerValue)
	case "task_state", "reboot_ordinal":
		return state.AcceptedCount >= uint64(fault.TriggerValue)
	default:
		return false
	}
}

func (h *Handler) abort(w http.ResponseWriter) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	conn, _, err := hijacker.Hijack()
	if err == nil {
		_ = conn.Close()
	}
}

func (h *Handler) checkpoint(ctx context.Context) error {
	if h.afterMutation == nil {
		return nil
	}
	return h.afterMutation(ctx)
}
func link(uri string) map[string]string { return map[string]string{"@odata.id": uri} }
func jsonOut(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func errorJSON(w http.ResponseWriter, status int, code, message string) {
	jsonOut(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
