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
