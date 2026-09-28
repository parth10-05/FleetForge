package inventory

import (
	"errors"
	"testing"
)

func TestSyntheticTargetAddressDerivesOnlyPublicServiceRoot(t *testing.T) {
	address, err := ParseSyntheticTargetAddress("HTTPS://SIMULATOR.EXAMPLE/", "rack-a-bmc-01", SyntheticContractV1)
	if err != nil {
		t.Fatalf("ParseSyntheticTargetAddress() error = %v", err)
	}
	if got, want := address.ServiceRoot(), "https://simulator.example/virtual-bmcs/rack-a-bmc-01/redfish/v1/"; got != want {
		t.Fatalf("ServiceRoot() = %q, want %q", got, want)
	}
	for _, input := range []struct{ origin, key, version string }{
		{"https://simulator.example", "../admin", SyntheticContractV1},
		{"https://simulator.example", "a/b", SyntheticContractV1},
		{"https://user:secret@simulator.example", "bmc1", SyntheticContractV1},
		{"https://simulator.example", "bmc1", "other"},
	} {
		_, err := ParseSyntheticTargetAddress(input.origin, input.key, input.version)
		if !errors.Is(err, ErrInvalidSyntheticTargetAddress) {
			t.Fatalf("ParseSyntheticTargetAddress(%+v) = %v", input, err)
		}
	}
}
