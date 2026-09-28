package redfishsim

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/simulator/core"
	"github.com/parth10-05/fleetforge/internal/simulator/journal"
	"github.com/parth10-05/fleetforge/internal/simulator/scenario"
)

func TestTargetRoutesRequireAuthAndExposeBoundedUpdate(t *testing.T) {
	s := scenario.Scenario{SchemaVersion: scenario.SchemaVersionV1, ScenarioID: "routes-six", Seed: "1", Topology: scenario.Topology{Sites: []string{"a", "b"}}, Targets: []scenario.Target{{Index: 1, Site: "a", Rack: "r1", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 2, Site: "a", Rack: "r2", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 3, Site: "b", Rack: "r3", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 4, Site: "b", Rack: "r4", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 5, Site: "b", Rack: "r4", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}, {Index: 6, Site: "a", Rack: "r2", InitialVersion: "1", ExpectedVersion: "2", CapabilityProfile: "baseline"}}, Timing: scenario.Timing{UploadMS: 1}, Limits: scenario.Limits{CallbackWorkers: 1, Subscriptions: 1, BodyBytes: 1024, JournalEntries: 1, OpenConnections: 1, EventsPerSecond: 1}}
	engine, err := core.Start(s, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	target, err := scenario.ID(s, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(t.TempDir(), "journal.jsonl")
	writer, err := journal.OpenWriter(journalPath, "run-routes")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	h := NewWithJournal(engine, []byte("user"), []byte("pass"), time.Second, 1024, writer).Routes()
	root := "/virtual-bmcs/" + target + "/redfish/v1"
	request := httptest.NewRequest(http.MethodGet, root, nil)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || response.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("unauthenticated response = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, root+"/UpdateService", nil)
	request.SetBasicAuth("user", "pass")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("update service = %d: %s", response.Code, response.Body.String())
	}
	body := []byte("synthetic-artifact")
	sum := sha256.Sum256(body)
	request = httptest.NewRequest(http.MethodPost, root+"/UpdateService/update", bytes.NewReader(body))
	request.SetBasicAuth("user", "pass")
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-FleetForge-Expected-Version", "2")
	request.Header.Set("X-FleetForge-Artifact-SHA256", hex.EncodeToString(sum[:]))
	request.Header.Set("X-FleetForge-Attempt", "attempt-one")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || response.Header().Get("Location") == "" {
		t.Fatalf("update = %d: %s", response.Code, response.Body.String())
	}
	if writer.SyncCount() != 2 {
		t.Fatalf("synced evidence records = %d, want 2", writer.SyncCount())
	}
	request = httptest.NewRequest(http.MethodGet, root+"/TaskService/Tasks", nil)
	request.SetBasicAuth("user", "pass")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("TaskService/Tasks")) {
		t.Fatalf("tasks = %d: %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, root+"/EventService/Subscriptions", bytes.NewBufferString(`{"Destination":"http://127.0.0.1:8088/events","Protocol":"Redfish","SubscriptionType":"RedfishEvent"}`))
	request.SetBasicAuth("user", "pass")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !bytes.Contains(response.Body.Bytes(), []byte("EventDestination")) {
		t.Fatalf("subscription = %d: %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, root+"/EventService/Subscriptions", nil)
	request.SetBasicAuth("user", "pass")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("Members@odata.count")) {
		t.Fatalf("subscriptions = %d: %s", response.Code, response.Body.String())
	}
}

func TestPostAcceptDisconnectPersistsExactlyOneAcceptance(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "fixtures", "simulator", "v1", "post-accept-disconnect.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := scenario.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := core.Start(s, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	target, err := scenario.ID(s, "target", 1)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	writer, err := journal.OpenWriter(path, "post-accept-run")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	server := httptest.NewServer(NewWithJournal(engine, []byte("user"), []byte("pass"), time.Second, 1024, writer).Routes())
	defer server.Close()
	body := []byte("synthetic-artifact")
	sum := sha256.Sum256(body)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/virtual-bmcs/"+target+"/redfish/v1/UpdateService/update", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth("user", "pass")
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-FleetForge-Expected-Version", "1.1")
	request.Header.Set("X-FleetForge-Artifact-SHA256", hex.EncodeToString(sum[:]))
	request.Header.Set("X-FleetForge-Attempt", "lost-response-attempt")
	response, err := server.Client().Do(request)
	if err == nil {
		_ = response.Body.Close()
		t.Fatal("post-accept request received an HTTP response, want connection loss")
	}
	state, err := engine.Submit(context.Background(), core.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Operations) != 1 || state.Operations[0].AttemptID != "lost-response-attempt" {
		t.Fatalf("operations after post-accept loss = %#v", state.Operations)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := journal.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := journal.ReplayAccepted(entries)
	if err != nil || len(accepted) != 1 {
		t.Fatalf("accepted journal entries = %#v, %v", accepted, err)
	}
}
