package redfish

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

type updateTestAuthenticator struct{}

func (updateTestAuthenticator) Apply(request *http.Request) {
	request.Header.Set("Authorization", "Bearer test")
}

type updateRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper updateRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func TestSubmitFirmwareAuthenticatedIssuesOnePushAndNormalizesTaskReference(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		if request.Method != http.MethodPost || request.URL.Path != "/virtual-bmcs/rack-a/redfish/v1/UpdateService/update" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-FleetForge-Artifact-SHA256") != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
			t.Fatalf("unexpected digest header")
		}
		writer.Header().Set("Location", "/virtual-bmcs/rack-a/redfish/v1/TaskService/Tasks/task-1")
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte(`{"@odata.id":"task-1"}`))
	}))
	defer server.Close()
	address, err := ParseTargetAddress(server.URL, "rack-a")
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(ClientConfig{HTTPClient: server.Client(), Total: time.Second, MaxBytes: 1024, AllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}})
	result := client.SubmitFirmwareAuthenticated(context.Background(), address, updateTestAuthenticator{}, FirmwareSubmitRequest{ActionPath: "UpdateService/update", ExpectedVersion: "2", ArtifactSHA256: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AttemptID: "attempt-1", Body: bytes.NewReader([]byte("firmware"))})
	if result.Class != FirmwareAccepted || result.RemoteTaskReference != "/redfish/v1/TaskService/Tasks/task-1" || result.ResponseFingerprint == "" || calls != 1 {
		t.Fatalf("result=%+v calls=%d", result, calls)
	}
}

func TestSubmitFirmwareAuthenticatedTreatsLostResponseAsPossiblyAccepted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		hijacker, ok := writer.(http.Hijacker)
		if !ok {
			t.Fatal("response writer cannot hijack")
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		_ = connection.Close()
	}))
	defer server.Close()
	address, err := ParseTargetAddress(server.URL, "rack-a")
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(ClientConfig{HTTPClient: server.Client(), Total: time.Second, MaxBytes: 1024})
	result := client.SubmitFirmwareAuthenticated(context.Background(), address, updateTestAuthenticator{}, FirmwareSubmitRequest{ActionPath: "UpdateService/update", ExpectedVersion: "2", ArtifactSHA256: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AttemptID: "attempt-1", Body: bytes.NewReader([]byte("firmware"))})
	if result.Class != FirmwarePossiblyAccepted {
		t.Fatalf("class=%s, want possibly accepted", result.Class)
	}
}

func TestSubmitFirmwareAuthenticatedClassifiesOnlyExplicitPrewriteProofAsRetryable(t *testing.T) {
	address, err := ParseTargetAddress("http://127.0.0.1:8080", "rack-a")
	if err != nil {
		t.Fatal(err)
	}
	request := FirmwareSubmitRequest{ActionPath: "UpdateService/update", ExpectedVersion: "2", ArtifactSHA256: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AttemptID: "attempt-1", Body: bytes.NewReader([]byte("firmware"))}
	provenClient := NewClient(ClientConfig{HTTPClient: &http.Client{Transport: updateRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, MarkPrewriteFailure(errors.New("dial rejected before write"))
	})}, Total: time.Second, MaxBytes: 1024})
	if result := provenClient.SubmitFirmwareAuthenticated(context.Background(), address, updateTestAuthenticator{}, request); result.Class != FirmwareProvenPrewriteFailure {
		t.Fatalf("proven class=%s, want %s", result.Class, FirmwareProvenPrewriteFailure)
	}
	uncertainClient := NewClient(ClientConfig{HTTPClient: &http.Client{Transport: updateRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport reset")
	})}, Total: time.Second, MaxBytes: 1024})
	if result := uncertainClient.SubmitFirmwareAuthenticated(context.Background(), address, updateTestAuthenticator{}, request); result.Class != FirmwarePossiblyAccepted {
		t.Fatalf("uncertain class=%s, want %s", result.Class, FirmwarePossiblyAccepted)
	}
}

func TestSubmitFirmwareAuthenticatedClassifiesFourXXAsExplicitRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { writer.WriteHeader(http.StatusBadRequest) }))
	defer server.Close()
	address, err := ParseTargetAddress(server.URL, "rack-a")
	if err != nil {
		t.Fatal(err)
	}
	result := NewClient(ClientConfig{HTTPClient: server.Client(), Total: time.Second, MaxBytes: 1024}).SubmitFirmwareAuthenticated(context.Background(), address, updateTestAuthenticator{}, FirmwareSubmitRequest{ActionPath: "UpdateService/update", ExpectedVersion: "2", ArtifactSHA256: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AttemptID: "attempt-1", Body: bytes.NewReader([]byte("firmware"))})
	if result.Class != FirmwareExplicitRejection || result.ResponseFingerprint == "" {
		t.Fatalf("result=%+v", result)
	}
}
