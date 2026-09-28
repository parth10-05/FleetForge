package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	appsim "github.com/parth10-05/fleetforge/internal/application/simulator"
	"github.com/parth10-05/fleetforge/internal/delivery/healthhttp"
	"github.com/parth10-05/fleetforge/internal/delivery/redfishsim"
	"github.com/parth10-05/fleetforge/internal/delivery/simulatoradmin"
	"github.com/parth10-05/fleetforge/internal/platform/buildinfo"
	"github.com/parth10-05/fleetforge/internal/platform/config"
	"github.com/parth10-05/fleetforge/internal/platform/simulatorstate"
	"github.com/parth10-05/fleetforge/internal/simulator/clock"
	"github.com/parth10-05/fleetforge/internal/simulator/core"
	"github.com/parth10-05/fleetforge/internal/simulator/journal"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

func main() {
	if err := run(); err != nil {
		slog.Error("redfish simulator stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadSimulator()
	if err != nil {
		return err
	}
	rawScenario, err := os.ReadFile(cfg.ScenarioPath)
	if err != nil {
		return fmt.Errorf("read simulator scenario: %w", err)
	}
	input, err := scenario.Decode(rawScenario)
	if err != nil {
		return fmt.Errorf("decode simulator scenario: %w", err)
	}
	run, err := simulatorstate.OpenOrCreateRun(cfg.RunRoot, input)
	if err != nil {
		return fmt.Errorf("open retained simulator run: %w", err)
	}
	release, err := run.AcquireOwner()
	if err != nil {
		return fmt.Errorf("acquire retained simulator run owner: %w", err)
	}
	defer release()
	engine, err := startRetainedEngine(run, input)
	if err != nil {
		return fmt.Errorf("start simulator engine: %w", err)
	}
	defer engine.Close()
	writer, err := journal.OpenWriterLimited(run.JournalPath, run.ID, uint64(input.Limits.JournalEntries))
	if err != nil {
		return fmt.Errorf("open retained simulator journal: %w", err)
	}
	defer writer.Close()
	health := healthhttp.NewHandler(healthhttp.FileProbe{Path: cfg.ReadinessFile}, cfg.ReadinessTimeout, buildinfo.Current())
	health.MarkStarted()
	mux := http.NewServeMux()
	targets := redfishsim.NewWithJournal(engine, []byte(cfg.Username), []byte(cfg.Password), cfg.ReadinessTimeout, int64(input.Limits.BodyBytes), writer)
	targets.SetAfterMutation(func(ctx context.Context) error { return checkpoint(run, engine, ctx) })
	targets.SetOpenConnectionLimit(input.Limits.OpenConnections)
	dispatcher := redfishsim.NewCallbackDispatcher(input.Limits.CallbackWorkers, input.Limits.OpenConnections, cfg.ReadinessTimeout)
	defer dispatcher.Close()
	targets.SetCallbackDispatcher(dispatcher)
	realtimeContext, stopRealtime := context.WithCancel(context.Background())
	realtimeDone := make(chan struct{})
	go advanceRealtime(realtimeContext, realtimeDone, engine, targets, run)
	defer func() { stopRealtime(); <-realtimeDone }()
	mux.Handle("/virtual-bmcs/", targets.Routes())
	mux.Handle("/", health.Routes())
	server := &http.Server{Addr: cfg.Address, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8 << 10}
	admin := &http.Server{Addr: cfg.AdminAddress, Handler: simulatoradmin.New(appsim.New(runtimeStatus{runID: run.ID, journalPath: run.JournalPath, run: run, engine: engine, writer: writer, targets: targets}), []byte(cfg.AdminSecret), cfg.ReadinessTimeout).Routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8 << 10}
	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errorsChannel := make(chan error, 2)
	go func() { errorsChannel <- server.ListenAndServe() }()
	go func() { errorsChannel <- admin.ListenAndServe() }()
	slog.Info("redfish simulator started", "role", "redfish-simulator", "environment", cfg.Environment, "address", cfg.Address, "version", buildinfo.Current().Version)
	select {
	case err := <-errorsChannel:
		stopRealtime()
		<-realtimeDone
		shutdownServers(server, admin, cfg.ShutdownTimeout)
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-shutdownContext.Done():
		stopRealtime()
		<-realtimeDone
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := checkpoint(run, engine, ctx); err != nil {
			return fmt.Errorf("checkpoint retained simulator state: %w", err)
		}
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		if err := admin.Shutdown(ctx); err != nil {
			return fmt.Errorf("graceful admin shutdown: %w", err)
		}
		return nil
	}
}

func advanceRealtime(ctx context.Context, done chan<- struct{}, engine *core.Engine, targets *redfishsim.Handler, run simulatorstate.Run) {
	defer close(done)
	initial, err := engine.Submit(ctx, core.Snapshot{})
	if err != nil {
		return
	}
	next := initial.Time + 1
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result, err := engine.Submit(ctx, core.AdvanceTo{Time: clock.LogicalTime(next)})
			if err != nil {
				return
			}
			next++
			if len(result.Fired) == 0 {
				continue
			}
			targets.DispatchEvents(ctx, result)
			_ = checkpoint(run, engine, context.Background())
		}
	}
}

func startRetainedEngine(run simulatorstate.Run, input scenario.Scenario) (*core.Engine, error) {
	snapshot, err := journal.ReadSnapshot(run.SnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("read retained simulator snapshot: %w", err)
	}
	if len(snapshot.State) > 0 && string(snapshot.State) != "null" {
		return core.StartFromSnapshot(input, snapshot.State, 64)
	}
	entries, err := journal.Load(run.JournalPath)
	if err != nil {
		return nil, fmt.Errorf("load retained simulator journal: %w", err)
	}
	return core.StartRecovered(input, entries, 64)
}

func checkpoint(run simulatorstate.Run, engine *core.Engine, ctx context.Context) error {
	state, result, err := core.SnapshotBytes(ctx, engine)
	if err != nil {
		return err
	}
	return run.WriteStateSnapshot(int64(result.Time), state)
}

func shutdownServers(server, admin *http.Server, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_ = server.Shutdown(ctx)
	_ = admin.Shutdown(ctx)
}

type runtimeStatus struct {
	runID, journalPath string
	run                simulatorstate.Run
	engine             *core.Engine
	writer             *journal.Writer
	targets            *redfishsim.Handler
}

func (r runtimeStatus) Status(context.Context) (appsim.Status, error) {
	return appsim.Status{RunID: r.runID, Ready: true}, nil
}
func (r runtimeStatus) Metrics(context.Context) (appsim.Metrics, error) {
	metrics := r.targets.Metrics()
	return appsim.Metrics{Requests: metrics.Requests, AcceptedUpdates: metrics.AcceptedUpdates, TaskObservations: metrics.TaskObservations, Faults: metrics.Faults}, nil
}
func (r runtimeStatus) Journal(context.Context) (appsim.JournalStatus, error) {
	entries, err := journal.Load(r.journalPath)
	if err != nil {
		return appsim.JournalStatus{}, err
	}
	status := appsim.JournalStatus{Entries: uint64(len(entries))}
	if len(entries) > 0 {
		status.LastHash = entries[len(entries)-1].Hash
	}
	return status, nil
}
func (r runtimeStatus) Reset(ctx context.Context) error {
	if _, err := r.engine.Submit(ctx, core.Reset{}); err != nil {
		return err
	}
	if _, err := r.writer.Append(journal.Record{Kind: journal.Diagnostic, Payload: journal.Payload{Detail: "admin reset"}}); err != nil {
		return err
	}
	return checkpoint(r.run, r.engine, ctx)
}
