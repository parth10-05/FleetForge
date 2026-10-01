package redfish

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"testing"
	"time"
)

type testAuthenticator struct{}

func (testAuthenticator) Apply(request *http.Request) { request.SetBasicAuth("user", "pass") }
func loopbackClientConfig() ClientConfig {
	return ClientConfig{HTTPClient: &http.Client{}}
}

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
	document, err := NewClient(loopbackClientConfig()).GetDocument(context.Background(), address, "Systems/1")
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
			_, err = NewClient(loopbackClientConfig()).GetDocument(context.Background(), address, "Systems/1")
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
	config := loopbackClientConfig()
	config.Total = time.Millisecond
	_, err = NewClient(config).GetDocument(context.Background(), address, "Systems/1")
	var failure DiscoveryError
	if !errors.As(err, &failure) || failure.Kind != ErrorTimeout {
		t.Fatalf("GetDocument error = %v, want timeout", err)
	}
}

func TestClientAppliesAuthenticatorWithoutChangingOriginRules(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, password, ok := request.BasicAuth()
		if !ok || user != "user" || password != "pass" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"Id":"one"}`))
	}))
	defer server.Close()
	address, _ := ParseTargetAddress(server.URL, "rack-a")
	if _, err := NewClient(loopbackClientConfig()).GetDocumentAuthenticated(context.Background(), address, "Systems/1", testAuthenticator{}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewClient(loopbackClientConfig()).GetDocumentAuthenticated(context.Background(), address, "Systems/1", nil); err == nil {
		t.Fatal("nil authenticator accepted")
	}
}

func TestDiscoveryErrorRetryabilityFailsClosed(t *testing.T) {
	for _, kind := range []ErrorKind{ErrorUnreachable, ErrorRateLimited, ErrorTransient, ErrorTimeout} {
		if !kind.Retryable() {
			t.Fatalf("%q should be retryable", kind)
		}
	}
	for _, kind := range []ErrorKind{ErrorUnauthenticated, ErrorUnauthorized, ErrorMalformed, ErrorUnsupported} {
		if kind.Retryable() {
			t.Fatalf("%q must not be retryable", kind)
		}
	}
}

func TestForbiddenDialPolicyIsMalformedNotRetryable(t *testing.T) {
	err := classifyTransportError(ErrForbiddenAddress)
	var failure DiscoveryError
	if !errors.As(err, &failure) || failure.Kind != ErrorMalformed || failure.Kind.Retryable() {
		t.Fatalf("forbidden dial classification = %v", err)
	}
}

func TestProductionDialBindsResolutionToAllowedCIDR(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{})
	go func() {
		connection, err := listener.Accept()
		if err == nil {
			connection.Close()
			close(accepted)
		}
	}()
	dial := dialApprovedIP([]netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	})
	connection, err := dial(context.Background(), "tcp", "simulator.example:"+strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
	<-accepted
	denied := dialApprovedIP(nil, func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("198.51.100.8")}, nil
	})
	if _, err := denied(context.Background(), "tcp", "simulator.example:443"); !errors.Is(err, ErrForbiddenAddress) {
		t.Fatalf("unapproved resolution error=%v", err)
	}
}

func TestAuthenticatedClientRefusesForbiddenResolutionBeforeDial(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{}, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			connection.Close()
			accepted <- struct{}{}
		}
	}()
	address, err := ParseTargetAddress("http://simulator.example:"+strconv.Itoa(listener.Addr().(*net.TCPAddr).Port), "rack-a")
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(ClientConfig{ResolveIP: func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}})
	_, err = client.GetDocumentAuthenticated(context.Background(), address, "Systems/1", testAuthenticator{})
	var failure DiscoveryError
	if !errors.As(err, &failure) || failure.Kind != ErrorMalformed || !errors.Is(err, ErrForbiddenAddress) {
		t.Fatalf("forbidden authenticated request error=%v", err)
	}
	select {
	case <-accepted:
		t.Fatal("forbidden authenticated request reached a listener")
	case <-time.After(100 * time.Millisecond):
	}
}
