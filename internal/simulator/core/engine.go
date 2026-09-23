// Package core owns the single-goroutine, deterministic virtual-BMC truth.
package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/parth10-05/fleetforge/internal/simulator/clock"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

var (
	ErrUnknownTarget     = errors.New("unknown synthetic target")
	ErrUnknownTask       = errors.New("unknown synthetic task")
	ErrUnknownFault      = errors.New("unknown synthetic fault")
	ErrFaultExhausted    = errors.New("synthetic fault is exhausted")
	ErrEngineClosed      = errors.New("simulator engine is closed")
	ErrInvalidTransition = errors.New("invalid synthetic transition")
	ErrLimitExceeded     = errors.New("synthetic limit exceeded")
)

type Lifecycle string

const (
	LifecycleAccepted             Lifecycle = "accepted"
	LifecycleRunning              Lifecycle = "running"
	LifecycleRebooting            Lifecycle = "rebooting"
	LifecycleInventoryPending     Lifecycle = "inventory-pending"
	LifecycleCompleted            Lifecycle = "completed"
	LifecycleHealthy              Lifecycle = "healthy"
	LifecycleException            Lifecycle = "exception"
	LifecycleTaskMissing          Lifecycle = "task-missing"
	LifecycleVersionContradiction Lifecycle = "version-contradiction"
	LifecycleHealthFailed         Lifecycle = "health-failed"
)
const (
	priorityTaskRunning = 10
	priorityRebooting   = 20
	priorityInventory   = 30
	priorityHealth      = 40
	priorityHealthy     = 45
)

type TargetState struct {
	ID, Site, Rack, Version, StagedVersion     string
	Healthy, Reachable                         bool
	Phase                                      Lifecycle
	RequestCount, AcceptedCount, EventSequence uint64
}
type OperationState struct {
	ID, TargetID, AttemptID, Version, TaskID string
	Ordinal                                  uint64
	State                                    Lifecycle
}
type TaskState struct {
	ID, TargetID, OperationID string
	State                     Lifecycle
	PercentComplete           int
	Visible                   bool
}
type SubscriptionState struct {
	ID, TargetID, Fingerprint string
	Ordinal                   uint64
}
type EventState struct {
	ID, TargetID, TaskID string
	Sequence             uint64
	Kind                 string
}
type FaultState struct {
	ID, Name, Scope, ScopeKey string
	Remaining, Priority       int
}
type Result struct {
	Sequence      uint64
	Time          clock.LogicalTime
	State         []TargetState
	Operations    []OperationState
	Tasks         []TaskState
	Subscriptions []SubscriptionState
	Events        []EventState
	Faults        []FaultState
	Fired         []clock.Event
}

type target struct {
	TargetState
	ExpectedVersion string
	subscriptions   uint64
}
type model struct {
	input                                                scenario.Scenario
	targets                                              map[string]target
	operations                                           map[string]OperationState
	tasks                                                map[string]TaskState
	subscriptions                                        map[string]SubscriptionState
	events                                               map[string]EventState
	faults                                               map[string]FaultState
	acceptanceOrdinal, subscriptionOrdinal, eventOrdinal uint64
}
type request struct {
	command Command
	result  chan response
}
type response struct {
	result Result
	err    error
}
type Engine struct {
	requests  chan request
	stop      chan struct{}
	stopped   chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
}

func Start(input scenario.Scenario, queueSize int) (*Engine, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if queueSize < 1 || queueSize > 4096 {
		return nil, fmt.Errorf("invalid engine queue size")
	}
	m := model{input: input, targets: map[string]target{}, operations: map[string]OperationState{}, tasks: map[string]TaskState{}, subscriptions: map[string]SubscriptionState{}, events: map[string]EventState{}, faults: map[string]FaultState{}}
	for _, v := range input.Targets {
		id, err := scenario.ID(input, "target", v.Index)
		if err != nil {
			return nil, err
		}
		m.targets[id] = target{TargetState: TargetState{ID: id, Site: v.Site, Rack: v.Rack, Version: v.InitialVersion, Healthy: true, Reachable: true, Phase: LifecycleHealthy}, ExpectedVersion: v.ExpectedVersion}
	}
	for _, v := range input.Faults {
		m.faults[v.FaultID] = FaultState{ID: v.FaultID, Name: v.Name, Scope: v.Scope, ScopeKey: v.ScopeKey, Remaining: v.Consumption, Priority: v.Priority}
	}
	return startModel(m, queueSize)
}

func startModel(m model, queueSize int) (*Engine, error) {
	if queueSize < 1 || queueSize > 4096 {
		return nil, fmt.Errorf("invalid engine queue size")
	}
	e := &Engine{requests: make(chan request, queueSize), stop: make(chan struct{}), stopped: make(chan struct{})}
	go e.loop(m)
	return e, nil
}
func (e *Engine) Close() {
	e.closeOnce.Do(func() { e.mu.Lock(); close(e.stop); e.mu.Unlock(); <-e.stopped })
}
func (e *Engine) Submit(ctx context.Context, c Command) (Result, error) {
	if c == nil {
		return Result{}, errors.New("nil simulator command")
	}
	reply := make(chan response, 1)
	r := request{c, reply}
	e.mu.Lock()
	select {
	case <-e.stop:
		e.mu.Unlock()
		return Result{}, ErrEngineClosed
	case <-ctx.Done():
		e.mu.Unlock()
		return Result{}, ctx.Err()
	case e.requests <- r:
		e.mu.Unlock()
	}
	select {
	case x := <-reply:
		return x.result, x.err
	case <-e.stopped:
		return Result{}, ErrEngineClosed
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}
func (e *Engine) loop(m model) {
	defer close(e.stopped)
	now := clock.NewManual(0)
	var schedule clock.Scheduler
	var seq uint64
	for {
		select {
		case <-e.stop:
			return
		case r := <-e.requests:
			seq++
			out, err := m.apply(r.command, now, &schedule, seq)
			r.result <- response{out, err}
		}
	}
}

func (m *model) apply(c Command, now *clock.Manual, s *clock.Scheduler, seq uint64) (Result, error) {
	var fired []clock.Event
	switch v := c.(type) {
	case Observe:
		if _, ok := m.targets[v.TargetID]; !ok {
			return Result{}, ErrUnknownTarget
		}
	case SetHealth:
		t, ok := m.targets[v.TargetID]
		if !ok {
			return Result{}, ErrUnknownTarget
		}
		t.Healthy = v.Healthy
		if !v.Healthy {
			t.Phase = LifecycleHealthFailed
		}
		m.targets[v.TargetID] = t
	case ScheduleReachability:
		if _, ok := m.targets[v.TargetID]; !ok {
			return Result{}, ErrUnknownTarget
		}
		if v.At < now.Now() {
			return Result{}, clock.ErrTimeReversal
		}
		value := "unreachable"
		if v.Reachable {
			value = "reachable"
		}
		s.Schedule(v.At, v.Priority, v.TargetID, value)
	case ScheduleTransition:
		if err := m.validateTransition(v); err != nil {
			return Result{}, err
		}
		if v.At < now.Now() {
			return Result{}, clock.ErrTimeReversal
		}
		s.Schedule(v.At, priority(v.Kind), v.TargetID, string(v.Kind)+":"+v.TaskID)
	case AcceptUpdate:
		if err := m.accept(v, now, s); err != nil {
			return Result{}, err
		}
	case CreateSubscription:
		if err := m.subscribe(v); err != nil {
			return Result{}, err
		}
	case ConsumeFault:
		if err := m.consume(v.TargetID, v.FaultID); err != nil {
			return Result{}, err
		}
	case AdvanceTo:
		if v.Time < now.Now() {
			return Result{}, clock.ErrTimeReversal
		}
		if err := now.AdvanceTo(v.Time); err != nil {
			return Result{}, err
		}
		for {
			due := s.PopDue(now.Now())
			if len(due) == 0 {
				break
			}
			fired = append(fired, due...)
			for _, event := range due {
				if err := m.fire(event, now, s); err != nil {
					return Result{}, err
				}
			}
		}
	default:
		return Result{}, errors.New("unsupported simulator command")
	}
	return m.snapshot(seq, now.Now(), fired), nil
}
func priority(k TransitionKind) int {
	switch k {
	case TransitionTaskRunning:
		return priorityTaskRunning
	case TransitionRebooting:
		return priorityRebooting
	case TransitionInventory:
		return priorityInventory
	case TransitionHealth:
		return priorityHealth
	}
	return -1
}
func (m *model) validateTransition(v ScheduleTransition) error {
	if _, ok := m.targets[v.TargetID]; !ok {
		return ErrUnknownTarget
	}
	task, ok := m.tasks[v.TaskID]
	if !ok {
		return ErrUnknownTask
	}
	if task.TargetID != v.TargetID || priority(v.Kind) < 0 {
		return ErrInvalidTransition
	}
	return nil
}
func (m *model) accept(v AcceptUpdate, now *clock.Manual, s *clock.Scheduler) error {
	t, ok := m.targets[v.TargetID]
	if !ok {
		return ErrUnknownTarget
	}
	if v.Version == "" || len(v.Version) > 256 || v.Version != t.ExpectedVersion || len(v.AttemptID) > 128 {
		return ErrInvalidTransition
	}
	t.RequestCount++
	t.AcceptedCount++
	t.StagedVersion = v.Version
	m.targets[v.TargetID] = t
	m.acceptanceOrdinal++
	op, err := scenario.ID(m.input, "operation", int(m.acceptanceOrdinal))
	if err != nil {
		return err
	}
	task, err := scenario.ID(m.input, "task", int(m.acceptanceOrdinal))
	if err != nil {
		return err
	}
	m.operations[op] = OperationState{ID: op, TargetID: v.TargetID, AttemptID: v.AttemptID, Version: v.Version, TaskID: task, Ordinal: m.acceptanceOrdinal, State: LifecycleAccepted}
	m.tasks[task] = TaskState{ID: task, TargetID: v.TargetID, OperationID: op, State: LifecycleAccepted, Visible: true}
	base := now.Now()
	tm := m.input.Timing
	s.Schedule(base+clock.LogicalTime(tm.UploadMS), priorityTaskRunning, v.TargetID, "task-running:"+task)
	s.Schedule(base+clock.LogicalTime(tm.UploadMS+tm.TaskStepMS), priorityRebooting, v.TargetID, "rebooting:"+task)
	s.Schedule(base+clock.LogicalTime(tm.UploadMS+tm.TaskStepMS+tm.RebootMS), priorityInventory, v.TargetID, "inventory:"+task)
	s.Schedule(base+clock.LogicalTime(tm.UploadMS+tm.TaskStepMS+tm.RebootMS+tm.InventoryVisibilityMS), priorityHealth, v.TargetID, "health:"+task)
	return nil
}
func (m *model) subscribe(v CreateSubscription) error {
	t, ok := m.targets[v.TargetID]
	if !ok {
		return ErrUnknownTarget
	}
	if v.Fingerprint == "" || len(v.Fingerprint) > 128 {
		return ErrInvalidTransition
	}
	if t.subscriptions >= uint64(m.input.Limits.Subscriptions) {
		return ErrLimitExceeded
	}
	m.subscriptionOrdinal++
	id, err := scenario.ID(m.input, "subscription", int(m.subscriptionOrdinal))
	if err != nil {
		return err
	}
	m.subscriptions[id] = SubscriptionState{ID: id, TargetID: v.TargetID, Fingerprint: v.Fingerprint, Ordinal: m.subscriptionOrdinal}
	t.subscriptions++
	m.targets[v.TargetID] = t
	return nil
}
func (m *model) fire(event clock.Event, now *clock.Manual, s *clock.Scheduler) error {
	if event.Value == "reachable" || event.Value == "unreachable" {
		t, ok := m.targets[event.TargetID]
		if !ok {
			return ErrUnknownTarget
		}
		t.Reachable = event.Value == "reachable"
		m.targets[event.TargetID] = t
		return nil
	}
	parts := strings.SplitN(event.Value, ":", 2)
	if len(parts) != 2 {
		return ErrInvalidTransition
	}
	task, ok := m.tasks[parts[1]]
	if !ok || task.TargetID != event.TargetID {
		return ErrUnknownTask
	}
	target := m.targets[event.TargetID]
	op := m.operations[task.OperationID]
	switch TransitionKind(parts[0]) {
	case TransitionTaskRunning:
		if task.State != LifecycleAccepted {
			return ErrInvalidTransition
		}
		task.State = LifecycleRunning
		task.PercentComplete = 10
		op.State = LifecycleRunning
	case TransitionRebooting:
		if task.State != LifecycleRunning {
			return ErrInvalidTransition
		}
		task.State = LifecycleRebooting
		task.PercentComplete = 50
		target.Reachable = false
		target.Phase = LifecycleRebooting
		op.State = LifecycleRebooting
	case TransitionInventory:
		if task.State != LifecycleRebooting {
			return ErrInvalidTransition
		}
		task.State = LifecycleInventoryPending
		task.PercentComplete = 90
		target.Reachable = true
		target.Phase = LifecycleInventoryPending
		target.Version = op.Version
		if target.StagedVersion == op.Version {
			target.StagedVersion = ""
		}
		op.State = LifecycleInventoryPending
		if id := m.firstFault(target.ID, "RF-F08"); id != "" {
			_ = m.consume(target.ID, id)
			task.Visible = false
			task.State = LifecycleTaskMissing
			op.State = LifecycleTaskMissing
		}
	case TransitionHealth:
		if task.State != LifecycleInventoryPending && task.State != LifecycleTaskMissing {
			return ErrInvalidTransition
		}
		if id := m.firstFault(target.ID, "RF-F10"); id != "" {
			_ = m.consume(target.ID, id)
			target.Version = m.initial(target.ID)
			target.Phase = LifecycleVersionContradiction
			task.State = LifecycleVersionContradiction
			op.State = LifecycleVersionContradiction
		} else if id := m.firstFault(target.ID, "RF-F11"); id != "" {
			_ = m.consume(target.ID, id)
			target.Healthy = false
			target.Phase = LifecycleHealthFailed
			task.State = LifecycleHealthFailed
			op.State = LifecycleHealthFailed
		} else {
			task.State = LifecycleCompleted
			task.PercentComplete = 100
			target.Phase = LifecycleCompleted
			op.State = LifecycleCompleted
			s.Schedule(now.Now(), priorityHealthy, target.ID, "healthy:"+task.ID)
		}
	case "healthy":
		if task.State != LifecycleCompleted {
			return ErrInvalidTransition
		}
		task.State = LifecycleHealthy
		target.Phase = LifecycleHealthy
		op.State = LifecycleHealthy
	default:
		return ErrInvalidTransition
	}
	m.tasks[task.ID] = task
	m.targets[target.ID] = target
	m.operations[op.ID] = op
	m.emit(target.ID, task.ID, parts[0])
	return nil
}
func (m *model) initial(id string) string {
	for _, v := range m.input.Targets {
		x, _ := scenario.ID(m.input, "target", v.Index)
		if x == id {
			return v.InitialVersion
		}
	}
	return ""
}
func (m *model) firstFault(targetID, name string) string {
	var a []FaultState
	for _, f := range m.faults {
		if f.Name == name && f.Remaining > 0 && m.matches(targetID, f) {
			a = append(a, f)
		}
	}
	sort.Slice(a, func(i, j int) bool {
		if a[i].Priority != a[j].Priority {
			return a[i].Priority < a[j].Priority
		}
		return a[i].ID < a[j].ID
	})
	if len(a) == 0 {
		return ""
	}
	return a[0].ID
}
func (m *model) matches(id string, f FaultState) bool {
	t := m.targets[id]
	switch f.Scope {
	case "all":
		return true
	case "target":
		return f.ScopeKey == id
	case "rack":
		return f.ScopeKey == t.Rack
	case "site":
		return f.ScopeKey == t.Site
	}
	return false
}
func (m *model) consume(targetID, faultID string) error {
	if _, ok := m.targets[targetID]; !ok {
		return ErrUnknownTarget
	}
	f, ok := m.faults[faultID]
	if !ok || !m.matches(targetID, f) {
		return ErrUnknownFault
	}
	if f.Remaining < 1 {
		return ErrFaultExhausted
	}
	f.Remaining--
	m.faults[faultID] = f
	return nil
}
func (m *model) emit(targetID, taskID, kind string) {
	m.eventOrdinal++
	id, err := scenario.ID(m.input, "event", int(m.eventOrdinal))
	if err != nil {
		return
	}
	t := m.targets[targetID]
	t.EventSequence++
	m.targets[targetID] = t
	m.events[id] = EventState{ID: id, TargetID: targetID, TaskID: taskID, Sequence: t.EventSequence, Kind: kind}
}
func (m *model) snapshot(seq uint64, time clock.LogicalTime, fired []clock.Event) Result {
	r := Result{Sequence: seq, Time: time, Fired: append([]clock.Event(nil), fired...)}
	for _, v := range m.targets {
		r.State = append(r.State, v.TargetState)
	}
	for _, v := range m.operations {
		r.Operations = append(r.Operations, v)
	}
	for _, v := range m.tasks {
		r.Tasks = append(r.Tasks, v)
	}
	for _, v := range m.subscriptions {
		r.Subscriptions = append(r.Subscriptions, v)
	}
	for _, v := range m.events {
		r.Events = append(r.Events, v)
	}
	for _, v := range m.faults {
		r.Faults = append(r.Faults, v)
	}
	sort.Slice(r.State, func(i, j int) bool { return r.State[i].ID < r.State[j].ID })
	sort.Slice(r.Operations, func(i, j int) bool { return r.Operations[i].ID < r.Operations[j].ID })
	sort.Slice(r.Tasks, func(i, j int) bool { return r.Tasks[i].ID < r.Tasks[j].ID })
	sort.Slice(r.Subscriptions, func(i, j int) bool { return r.Subscriptions[i].ID < r.Subscriptions[j].ID })
	sort.Slice(r.Events, func(i, j int) bool { return r.Events[i].ID < r.Events[j].ID })
	sort.Slice(r.Faults, func(i, j int) bool { return r.Faults[i].ID < r.Faults[j].ID })
	return r
}
