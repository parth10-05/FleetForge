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
	"strings"
	"sync"
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

// Routes preserves the non-composed isolation boundary used by existing tests.
func Routes() http.Handler { return http.MaxBytesHandler(http.NewServeMux(), 4<<10) }
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/virtual-bmcs/", h.handle)
	return http.MaxBytesHandler(mux, h.maxBody)
}

func (h *Handler) handle(w http.ResponseWriter, r *http.Request) {
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
	if r.Method != http.MethodGet {
		errorJSON(w, http.StatusMethodNotAllowed, "Base.1.0.ActionNotSupported", "The requested synthetic method is not supported.")
		return
	}
	prefix := "/virtual-bmcs/" + target + "/redfish/v1"
	switch resource {
	case "":
		jsonOut(w, http.StatusOK, map[string]any{"@odata.type": "#ServiceRoot.v1_9_4.ServiceRoot", "Id": "RootService", "Systems": link(prefix + "/Systems"), "Managers": link(prefix + "/Managers"), "Chassis": link(prefix + "/Chassis"), "UpdateService": link(prefix + "/UpdateService"), "TaskService": link(prefix + "/TaskService"), "EventService": link(prefix + "/EventService"), "Oem": map[string]any{"FleetForge": map[string]bool{"Synthetic": true}}})
	case "Systems":
		jsonOut(w, http.StatusOK, map[string]any{"Members": []any{link(prefix + "/Systems/1")}, "Members@odata.count": 1})
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
	default:
		errorJSON(w, http.StatusNotFound, "Base.1.0.ResourceMissingAtURI", "The requested synthetic resource does not exist.")
	}
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
	uri := "/virtual-bmcs/" + target + "/redfish/v1/TaskService/Tasks/" + op.TaskID
	w.Header().Set("Location", uri)
	jsonOut(w, http.StatusAccepted, map[string]any{"@odata.id": uri, "Oem": map[string]any{"FleetForge": map[string]string{"Attempt": attempt}}})
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
