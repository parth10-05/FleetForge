package redfish

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverServiceNormalizesOnlyApprovedCapabilityLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"RedfishVersion":"1.16.0","Systems":{"@odata.id":"Systems"},"Managers":{"@odata.id":"Managers"},"UpdateService":{"@odata.id":"UpdateService"}}`))
	}))
	defer server.Close()
	address, _ := ParseTargetAddress(server.URL, "rack-a")
	got, err := NewClient(ClientConfig{}).DiscoverService(context.Background(), address)
	if err != nil || got.SystemsURI != "Systems" || got.RedfishVersion != "1.16.0" {
		t.Fatalf("discovery=%+v err=%v", got, err)
	}
}

func TestDiscoverServiceRejectsMissingOrEscapingCapabilityLink(t *testing.T) {
	for _, body := range []string{
		`{"RedfishVersion":"1","Systems":{},"Managers":{"@odata.id":"Managers"},"UpdateService":{"@odata.id":"UpdateService"}}`,
		`{"RedfishVersion":"1","Systems":{"@odata.id":"https://evil.example"},"Managers":{"@odata.id":"Managers"},"UpdateService":{"@odata.id":"UpdateService"}}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		address, _ := ParseTargetAddress(server.URL, "rack-a")
		_, err := NewClient(ClientConfig{}).DiscoverService(context.Background(), address)
		server.Close()
		var failure DiscoveryError
		if !errors.As(err, &failure) || failure.Kind != ErrorMalformed {
			t.Fatalf("error=%v", err)
		}
	}
}
