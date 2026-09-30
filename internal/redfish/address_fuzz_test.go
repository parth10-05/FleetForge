package redfish

import (
	"strings"
	"testing"
)

func FuzzTargetAddressKeepsAcceptedResourcesUnderServiceRoot(f *testing.F) {
	for _, seed := range []struct{ origin, key, resource string }{
		{"https://simulator.example", "rack-a", "/Systems"},
		{"https://simulator.example/", "rack-a", "/virtual-bmcs/rack-a/redfish/v1/Systems"},
		{"https://user:secret@simulator.example", "rack-a", "/Systems"},
		{"https://simulator.example", "rack-a", "https://evil.example/Systems"},
		{"https://simulator.example", "rack-a", "../../admin"},
	} {
		f.Add(seed.origin, seed.key, seed.resource)
	}
	f.Fuzz(func(t *testing.T, origin, key, resource string) {
		address, err := ParseTargetAddress(origin, key)
		if err != nil {
			return
		}
		resolved, err := address.ResolveResource(resource)
		if err != nil {
			return
		}
		if !strings.HasPrefix(resolved, address.ServiceRoot()) {
			t.Fatalf("accepted resource escaped service root: origin=%q key=%q resource=%q resolved=%q root=%q", origin, key, resource, resolved, address.ServiceRoot())
		}
	})
}
