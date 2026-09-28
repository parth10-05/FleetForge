package syntheticcredentials

import (
	"errors"
	"net/http/httptest"
	"testing"
)

func TestResolverAcceptsOnlyKnownSyntheticTestReference(t *testing.T) {
	credential, err := NewCredential("sim-user", "not-for-logs")
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := New(map[string]Credential{"rack-a": credential})
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolver.Resolve("synthetic-test:rack-a")
	if err != nil {
		t.Fatalf("Resolve() = %#v, %v", got, err)
	}
	request := httptest.NewRequest("GET", "https://sim.example", nil)
	got.Apply(request)
	username, password, ok := request.BasicAuth()
	if !ok || username != "sim-user" || password != "not-for-logs" {
		t.Fatal("resolved credential was not applied as Basic authentication")
	}
	for _, reference := range []string{"", "aws-sm:prod/rack-a", "synthetic-test:missing", "synthetic-test:bad/name"} {
		_, err := resolver.Resolve(reference)
		if !errors.Is(err, ErrInvalidReference) && !errors.Is(err, ErrUnknownReference) {
			t.Fatalf("Resolve(%q) error = %v", reference, err)
		}
	}
}

func TestResolverRejectsCredentialValuesThatCouldInjectHeaders(t *testing.T) {
	if _, err := NewCredential("user\r\nX-Header: unsafe", "password"); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("NewCredential() error = %v, want invalid credential", err)
	}
}
