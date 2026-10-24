// Package eventhinthttp exposes the narrow authenticated event-hint ingress.
package eventhinthttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/eventhint"
)

const SignatureHeader = "X-FleetForge-Event-Signature"

type Store interface {
	RecordEventHint(context.Context, eventhint.Hint) (string, error)
	QuarantineEventHint(context.Context, []byte, string, time.Time) error
}

type Handler struct {
	secret []byte
	store  Store
	now    func() time.Time
}

func New(secret []byte, store Store, now func() time.Time) *Handler {
	if now == nil {
		now = time.Now
	}
	return &Handler{secret: append([]byte(nil), secret...), store: store, now: now}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/events/synthetic", h.receive)
	return mux
}

func (h *Handler) receive(w http.ResponseWriter, r *http.Request) {
	if h.store == nil || len(h.secret) < 32 {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8193)
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 || len(body) > 8192 {
		h.quarantine(r.Context(), body, "INVALID_BODY")
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}
	now := h.now().UTC()
	hint, err := eventhint.Parse(h.secret, r.Header.Get(SignatureHeader), body, now)
	if err != nil {
		h.quarantine(r.Context(), body, "REJECTED")
		http.Error(w, "invalid event", http.StatusUnauthorized)
		return
	}
	if _, err = h.store.RecordEventHint(r.Context(), hint); err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) quarantine(ctx context.Context, body []byte, reason string) {
	if len(body) == 0 {
		body = []byte(hex.EncodeToString(sha256.New().Sum(nil)))
	}
	_ = h.store.QuarantineEventHint(ctx, body, reason, h.now().UTC())
}
