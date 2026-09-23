// Package core owns the single-goroutine, in-memory simulator truth.
package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/parth10-05/fleetforge/internal/simulator/clock"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

var (
	ErrUnknownTarget = errors.New("unknown synthetic target")
	ErrEngineClosed  = errors.New("simulator engine is closed")
)

type TargetState struct {
	ID, Site, Rack, Version string
	Healthy, Reachable      bool
}
type Result struct {
	Sequence uint64
	Time     clock.LogicalTime
	State    []TargetState
	Fired    []clock.Event
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

// Start validates the scenario, derives stable target IDs, and starts one
// owning goroutine. It has no filesystem, HTTP, callback, or wall-clock use.
func Start(input scenario.Scenario, queueSize int) (*Engine, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if queueSize < 1 || queueSize > 4096 {
		return nil, fmt.Errorf("invalid engine queue size")
	}
	engine := &Engine{requests: make(chan request, queueSize), stop: make(chan struct{}), stopped: make(chan struct{})}
	states := make(map[string]TargetState, len(input.Targets))
	for _, target := range input.Targets {
		id, err := scenario.ID(input, "target", target.Index)
		if err != nil {
			return nil, err
		}
		states[id] = TargetState{ID: id, Site: target.Site, Rack: target.Rack, Version: target.InitialVersion, Healthy: true, Reachable: true}
	}
	go engine.loop(states)
	return engine, nil
}

func (e *Engine) Close() {
	e.closeOnce.Do(func() { e.mu.Lock(); close(e.stop); e.mu.Unlock(); <-e.stopped })
}

func (e *Engine) Submit(ctx context.Context, command Command) (Result, error) {
	if command == nil {
		return Result{}, errors.New("nil simulator command")
	}
	result := make(chan response, 1)
	request := request{command: command, result: result}
	e.mu.Lock()
	select {
	case <-e.stop:
		e.mu.Unlock()
		return Result{}, ErrEngineClosed
	case <-ctx.Done():
		e.mu.Unlock()
		return Result{}, ctx.Err()
	case e.requests <- request:
		e.mu.Unlock()
	}
	select {
	case response := <-result:
		return response.result, response.err
	case <-e.stopped:
		return Result{}, ErrEngineClosed
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

func (e *Engine) loop(states map[string]TargetState) {
	defer close(e.stopped)
	manual := clock.NewManual(0)
	var scheduler clock.Scheduler
	var sequence uint64
	for {
		var request request
		select {
		case <-e.stop:
			return
		case request = <-e.requests:
		}
		sequence++
		result, err := apply(request.command, manual, &scheduler, states, sequence)
		request.result <- response{result: result, err: err}
	}
}

func apply(command Command, manual *clock.Manual, scheduler *clock.Scheduler, states map[string]TargetState, sequence uint64) (Result, error) {
	fired := []clock.Event{}
	switch command := command.(type) {
	case AdvanceTo:
		if err := manual.AdvanceTo(command.Time); err != nil {
			return Result{}, err
		}
		fired = scheduler.PopDue(manual.Now())
		for _, event := range fired {
			state, found := states[event.TargetID]
			if !found {
				return Result{}, ErrUnknownTarget
			}
			state.Reachable = event.Value == "reachable"
			states[event.TargetID] = state
		}
	case SetHealth:
		state, found := states[command.TargetID]
		if !found {
			return Result{}, ErrUnknownTarget
		}
		state.Healthy = command.Healthy
		states[command.TargetID] = state
	case ScheduleReachability:
		if _, found := states[command.TargetID]; !found {
			return Result{}, ErrUnknownTarget
		}
		if command.At < manual.Now() {
			return Result{}, clock.ErrTimeReversal
		}
		value := "unreachable"
		if command.Reachable {
			value = "reachable"
		}
		scheduler.Schedule(command.At, command.Priority, command.TargetID, value)
	default:
		return Result{}, errors.New("unsupported simulator command")
	}
	return Result{Sequence: sequence, Time: manual.Now(), State: snapshot(states), Fired: fired}, nil
}

func snapshot(states map[string]TargetState) []TargetState {
	values := make([]TargetState, 0, len(states))
	for _, state := range states {
		values = append(values, state)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values
}
