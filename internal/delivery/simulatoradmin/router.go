// Package simulatoradmin serves authenticated simulator-only administration.
package simulatoradmin

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/simulator"
)

const credentialHeader = "X-FleetForge-Simulator-Admin"

type Handler struct {
	service *simulator.Service
	secret  []byte
	timeout time.Duration
}

func New(service *simulator.Service, secret []byte, timeout time.Duration) *Handler {
	return &Handler{service: service, secret: append([]byte(nil), secret...), timeout: timeout}
}
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/v1/status", h.status)
	mux.HandleFunc("GET /admin/v1/metrics", h.metrics)
	mux.HandleFunc("GET /admin/v1/journal", h.journal)
	mux.HandleFunc("POST /admin/v1/reset", h.reset)
	return http.MaxBytesHandler(mux, 4<<10)
}
func (h *Handler) reset(writer http.ResponseWriter, request *http.Request) {
	if !h.authorized(request) {
		write(writer, http.StatusUnauthorized, map[string]string{"status": "denied"})
		return
	}
	ctx, cancel := contextWithTimeout(request, h.timeout)
	defer cancel()
	if err := h.service.Reset(ctx); err != nil {
		write(writer, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	write(writer, http.StatusAccepted, map[string]string{"status": "reset"})
}
func (h *Handler) journal(writer http.ResponseWriter, request *http.Request) {
	if !h.authorized(request) {
		write(writer, http.StatusUnauthorized, map[string]string{"status": "denied"})
		return
	}
	ctx, cancel := contextWithTimeout(request, h.timeout)
	defer cancel()
	state, err := h.service.Journal(ctx)
	if err != nil {
		write(writer, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	write(writer, http.StatusOK, state)
}
func (h *Handler) metrics(writer http.ResponseWriter, request *http.Request) {
	if !h.authorized(request) {
		write(writer, http.StatusUnauthorized, map[string]string{"status": "denied"})
		return
	}
	ctx, cancel := contextWithTimeout(request, h.timeout)
	defer cancel()
	metrics, err := h.service.Metrics(ctx)
	if err != nil {
		write(writer, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	write(writer, http.StatusOK, metrics)
}

func (h *Handler) status(writer http.ResponseWriter, request *http.Request) {
	if !h.authorized(request) {
		write(writer, http.StatusUnauthorized, map[string]string{"status": "denied"})
		return
	}
	ctx, cancel := contextWithTimeout(request, h.timeout)
	defer cancel()
	status, err := h.service.Status(ctx)
	if err != nil {
		write(writer, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	write(writer, http.StatusOK, status)
}

func (h *Handler) authorized(request *http.Request) bool {
	provided := []byte(request.Header.Get(credentialHeader))
	if len(h.secret) == 0 || len(provided) != len(h.secret) {
		return false
	}
	return subtle.ConstantTimeCompare(provided, h.secret) == 1
}
func write(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}
