package localobjecthttp

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/parth10-05/fleetforge/internal/domain/artifact"
	"github.com/parth10-05/fleetforge/internal/platform/localartifactstore"
)

const GrantHeader = "X-FleetForge-Upload-Grant"

type Handler struct {
	store      *localartifactstore.DiskStore
	secret     []byte
	now        func() time.Time
	completion interface {
		CompleteUpload(context.Context, string, artifact.ObjectIdentity) error
	}
}

func New(store *localartifactstore.DiskStore, secret []byte, now func() time.Time) *Handler {
	if now == nil {
		now = time.Now
	}
	return &Handler{store: store, secret: append([]byte(nil), secret...), now: now}
}
func (h *Handler) WithCompletion(completion interface {
	CompleteUpload(context.Context, string, artifact.ObjectIdentity) error
}) *Handler {
	h.completion = completion
	return h
}
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /v1/objects/upload", h.upload)
	return mux
}
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	parts := strings.Split(r.Header.Get(GrantHeader), ".")
	if len(parts) != 2 {
		http.Error(w, "invalid upload grant", http.StatusForbidden)
		return
	}
	grant := localartifactstore.SignedGrant{Payload: parts[0], Signature: parts[1]}
	claims, err := grant.Verify(h.secret, h.now().UTC())
	if err != nil || r.Header.Get("Content-Type") != claims.MediaType {
		http.Error(w, "invalid upload grant", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, claims.Size+1)
	identity, err := h.store.PutSignedGrant(h.secret, grant, h.now().UTC(), r.Body)
	if err != nil {
		http.Error(w, "upload rejected", http.StatusBadRequest)
		return
	}
	if h.completion != nil {
		if err := h.completion.CompleteUpload(r.Context(), claims.ArtifactID, identity); err != nil {
			http.Error(w, "upload receipt pending", http.StatusServiceUnavailable)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"version_id":"` + identity.VersionID() + `"}`))
}
