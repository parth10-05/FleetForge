package redfishexecution

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/execution"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
	"github.com/parth10-05/fleetforge/internal/redfish"
)

type targetLoader struct{ target Target }

func (loader targetLoader) LoadExecutionTarget(context.Context, string) (Target, error) {
	return loader.target, nil
}

func TestAdapterSubmitsUsingResolvedTargetAndMapsAcceptedResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") == "" {
			t.Fatal("missing credential")
		}
		writer.Header().Set("Location", "/virtual-bmcs/node-a/redfish/v1/TaskService/Tasks/task-1")
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	credential, err := syntheticcredentials.NewCredential("user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := syntheticcredentials.New(map[string]syntheticcredentials.Credential{"node-a": credential})
	if err != nil {
		t.Fatal(err)
	}
	adapter := Adapter{Loader: targetLoader{target: Target{Origin: server.URL, TargetKey: "node-a", CredentialReference: "synthetic-test:node-a"}}, Client: redfish.NewClient(redfish.ClientConfig{HTTPClient: server.Client(), Total: time.Second, MaxBytes: 1024}), Resolver: resolver}
	request := execution.SubmitRequest{Attempt: execution.AttemptSnapshot{AttemptID: "attempt-1", TargetID: "target-1", ExpectedVersion: "2", Artifact: execution.ObjectIdentity{SHA256: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}, UpdateActionPath: "UpdateService/update"}
	result := adapter.Submit(context.Background(), request, bytes.NewReader([]byte("firmware")))
	if result.Class != execution.SubmitAccepted || result.RemoteTaskReference != "/redfish/v1/TaskService/Tasks/task-1" {
		t.Fatalf("result=%+v", result)
	}
}

func TestAdapterTreatsCredentialResolutionFailureAsUnsafe(t *testing.T) {
	adapter := Adapter{Loader: targetLoader{target: Target{Origin: "http://localhost", TargetKey: "node-a", CredentialReference: "synthetic-test:missing"}}, Client: redfish.NewClient(redfish.ClientConfig{Total: time.Second}), Resolver: nil}
	result := adapter.Submit(context.Background(), execution.SubmitRequest{Attempt: execution.AttemptSnapshot{AttemptID: "attempt-1", TargetID: "target-1", ExpectedVersion: "2", Artifact: execution.ObjectIdentity{SHA256: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}, UpdateActionPath: "UpdateService/update"}, bytes.NewReader([]byte("firmware")))
	if result.Class != execution.SubmitUnsafeResponse {
		t.Fatalf("result=%+v", result)
	}
}

func TestAdapterCollectsTaskInventoryAndHealthEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/virtual-bmcs/node-a/redfish/v1/TaskService/Tasks/task-1":
			_, _ = writer.Write([]byte(`{"TaskState":"Completed"}`))
		case "/virtual-bmcs/node-a/redfish/v1/UpdateService/FirmwareInventory/BMC":
			_, _ = writer.Write([]byte(`{"Version":"2"}`))
		case "/virtual-bmcs/node-a/redfish/v1/Managers/1":
			_, _ = writer.Write([]byte(`{"Status":{"Health":"OK"}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	credential, _ := syntheticcredentials.NewCredential("user", "pass")
	resolver, _ := syntheticcredentials.New(map[string]syntheticcredentials.Credential{"node-a": credential})
	adapter := Adapter{Loader: targetLoader{target: Target{Origin: server.URL, TargetKey: "node-a", CredentialReference: "synthetic-test:node-a"}}, Client: redfish.NewClient(redfish.ClientConfig{HTTPClient: server.Client(), Total: time.Second, MaxBytes: 1024}), Resolver: resolver}
	attempt := execution.AttemptSnapshot{AttemptID: "attempt", TargetID: "target", TargetUpdateID: "update", ExpectedVersion: "2", RemoteTaskReference: "/redfish/v1/TaskService/Tasks/task-1"}
	task, err := adapter.ObserveTask(context.Background(), execution.TaskObservationRequest{Attempt: attempt})
	if err != nil || task.Kind != execution.ObservationTask || task.ContradictsExpectedState || task.EvidenceFingerprint == "" {
		t.Fatalf("task=%+v err=%v", task, err)
	}
	inventory, err := adapter.ObserveInventory(context.Background(), execution.InventoryObservationRequest{Attempt: attempt})
	if err != nil || inventory.ContradictsExpectedState {
		t.Fatalf("inventory=%+v err=%v", inventory, err)
	}
	health, err := adapter.ObserveHealth(context.Background(), execution.HealthObservationRequest{Attempt: attempt})
	if err != nil || health.ContradictsExpectedState {
		t.Fatalf("health=%+v err=%v", health, err)
	}
}
