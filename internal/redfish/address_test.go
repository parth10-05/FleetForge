package redfish

import (
	"errors"
	"net/netip"
	"testing"
)

func TestTargetAddressRejectsOriginEscapeBeforeIO(t *testing.T) {
	address, err := ParseTargetAddress("https://sim.example", "rack-a")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := address.ResolveResource("Systems/1"); err != nil || got != "https://sim.example/virtual-bmcs/rack-a/redfish/v1/Systems/1" {
		t.Fatalf("resource = %q, %v", got, err)
	}
	if got, err := address.ResolveResource("/virtual-bmcs/rack-a/redfish/v1/Systems/1"); err != nil || got != "https://sim.example/virtual-bmcs/rack-a/redfish/v1/Systems/1" {
		t.Fatalf("canonical absolute service path = %q, %v", got, err)
	}
	for _, raw := range []string{"https://evil.example/x", "//evil.example/x", "../admin", "%2e%2e/admin", "Systems\\admin", "Systems/1?token=x"} {
		if _, err := address.ResolveResource(raw); !errors.Is(err, ErrForbiddenAddress) {
			t.Fatalf("ResolveResource(%q) = %v", raw, err)
		}
	}
}

func TestTargetAddressRejectsOriginsWithAnEmptyQueryMarker(t *testing.T) {
	if _, err := ParseTargetAddress("https://sim.example?", "rack-a"); !errors.Is(err, ErrForbiddenAddress) {
		t.Fatalf("empty-query origin error = %v", err)
	}
}

func TestTargetAddressResolvesSimulatorTaskReference(t *testing.T) {
	address, err := ParseTargetAddress("http://127.0.0.1:12345", "target-dabb923d2f052386b8787839704b1e503a799340b5b6d76f0783480e79fa1d25")
	if err != nil {
		t.Fatal(err)
	}
	resource := "TaskService/Tasks/task-3acf57d3777a10ef5eaa23cb432e0929a720b02e505a74e8e60ac0e82dcfecc9"
	want := "http://127.0.0.1:12345/virtual-bmcs/target-dabb923d2f052386b8787839704b1e503a799340b5b6d76f0783480e79fa1d25/redfish/v1/" + resource
	if got, err := address.ResolveResource(resource); err != nil || got != want {
		t.Fatalf("ResolveResource(%q) = %q, %v; want %q", resource, got, err, want)
	}
}

func TestDialPolicyRejectsPrivateMetadataAndLoopback(t *testing.T) {
	allowed := []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")}
	if err := ValidateDialIP("198.51.100.8", allowed); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1"} {
		if !errors.Is(ValidateDialIP(raw, allowed), ErrForbiddenAddress) {
			t.Fatalf("ValidateDialIP(%q) allowed", raw)
		}
	}
	if err := ValidateDialIP("10.0.0.1", []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24")}); err != nil {
		t.Fatalf("explicitly allowlisted address rejected: %v", err)
	}
}

func TestDialPolicyMatchesIPv4MappedAddressAgainstIPv4Allowlist(t *testing.T) {
	if err := ValidateDialIP("::ffff:127.0.0.1", []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}); err != nil {
		t.Fatalf("mapped loopback rejected: %v", err)
	}
}
