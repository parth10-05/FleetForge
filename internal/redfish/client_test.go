package redfish

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientAcceptsOnlyBoundedJSONAtTargetServiceRoot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/virtual-bmcs/rack-a/redfish/v1/Systems/1" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = writer.Write([]byte(`{"Id":"1","Status":{"State":"Enabled"}}`))
	}))
	defer server.Close()

	address, err := ParseTargetAddress(server.URL, "rack-a")
	if err != nil {
		t.Fatal(err)
	}
	document, err := NewClient(ClientConfig{}).GetDocument(context.Background(), address, "Systems/1")
	if err != nil {
		t.Fatal(err)
	}
	if string(document) != `{"Id":"1","Status":{"State":"Enabled"}}` {
		t.Fatalf("document = %q", document)
	}
}

func TestClientDeniesRedirectAndMalformedResponses(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    ErrorKind
	}{
		{name: "redirect", want: ErrorMalformed, handler: func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, "https://evil.example", http.StatusFound)
		}},
		{name: "wrong type", want: ErrorMalformed, handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = writer.Write([]byte(`{}`))
		}},
		{name: "duplicate key", want: ErrorMalformed, handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"Id":"one","Id":"two"}`))
		}},
		{name: "rate limited", want: ErrorRateLimited, handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusTooManyRequests)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()
			address, err := ParseTargetAddress(server.URL, "rack-a")
			if err != nil {
				t.Fatal(err)
			}
			_, err = NewClient(ClientConfig{}).GetDocument(context.Background(), address, "Systems/1")
			var failure DiscoveryError
			if !errors.As(err, &failure) || failure.Kind != test.want {
				t.Fatalf("GetDocument error = %v, want kind %q", err, test.want)
			}
		})
	}
}

func TestClientHonorsTotalDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()
	address, err := ParseTargetAddress(server.URL, "rack-a")
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewClient(ClientConfig{Total: time.Millisecond}).GetDocument(context.Background(), address, "Systems/1")
	var failure DiscoveryError
	if !errors.As(err, &failure) || failure.Kind != ErrorTimeout {
		t.Fatalf("GetDocument error = %v, want timeout", err)
	}
}
