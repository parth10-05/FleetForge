package redfishsim

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/simulator/core"
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
	h := New(engine, []byte("user"), []byte("pass"), time.Second, 1024).Routes()
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
	request = httptest.NewRequest(http.MethodGet, root+"/TaskService/Tasks", nil)
	request.SetBasicAuth("user", "pass")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("TaskService/Tasks")) {
		t.Fatalf("tasks = %d: %s", response.Code, response.Body.String())
	}
}
