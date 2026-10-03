// Package healthhttp implements the operational HTTP contract.
package healthhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/parth10-05/fleetforge/internal/platform/buildinfo"
)

// ErrProbeUnavailable is a safe readiness reason code.
var ErrProbeUnavailable = errors.New("required probe unavailable")

type Probe interface{ Check(context.Context) error }

// ReasonedProbe maps a private probe failure to a stable, safe readiness
// reason. It lets persistence distinguish unavailable infrastructure from an
// incompatible schema without exposing connection details.
type ReasonedProbe interface {
	Probe
	ReadinessReason(error) string
}

// FileProbe requires an optional local prerequisite file. It keeps the foundation
// readiness testable without requiring a cloud or database dependency.
type FileProbe struct{ Path string }

func (p FileProbe) Check(_ context.Context) error {
	if p.Path == "" {
		return nil
	}
	info, err := os.Stat(p.Path)
	if err != nil || info.IsDir() {
		return ErrProbeUnavailable
	}
	return nil
}

// Handler exposes liveness, readiness, startup, and safe build metadata.
type Handler struct {
	probe   Probe
	timeout time.Duration
	build   buildinfo.Info
	mu      sync.RWMutex
	started bool
}

func NewHandler(probe Probe, timeout time.Duration, build buildinfo.Info) *Handler {
	return &Handler{probe: probe, timeout: timeout, build: build}
}
func (h *Handler) MarkStarted() { h.mu.Lock(); defer h.mu.Unlock(); h.started = true }
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", h.live)
	mux.HandleFunc("GET /health/ready", h.ready)
	mux.HandleFunc("GET /health/startup", h.startup)
	mux.HandleFunc("GET /version", h.version)
	return http.MaxBytesHandler(mux, 4<<10)
}
func (h *Handler) live(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}
func (h *Handler) ready(writer http.ResponseWriter, _ *http.Request) {
	if !h.isStarted() {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "reason": "startup_incomplete"})
		return
	}
	if h.probe != nil {
		ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
		defer cancel()
		if err := h.probe.Check(ctx); err != nil {
			reason := "required_probe_unavailable"
			if probe, ok := h.probe.(ReasonedProbe); ok {
				reason = probe.ReadinessReason(err)
			}
			writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "reason": reason})
			return
		}
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}
func (h *Handler) startup(writer http.ResponseWriter, _ *http.Request) {
	if !h.isStarted() {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"status": "starting"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}
func (h *Handler) version(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, h.build)
}
func (h *Handler) isStarted() bool { h.mu.RLock(); defer h.mu.RUnlock(); return h.started }
func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}
