package value

import (
	"errors"
	"strings"
	"testing"
)

func TestSHA256DigestRequiresCanonicalAlgorithmAndCase(t *testing.T) {
	digestText := "sha256:" + strings.Repeat("a", 64)
	digest, err := ParseSHA256Digest(digestText)
	if err != nil || digest.String() != digestText {
		t.Fatalf("ParseSHA256Digest() = %q, %v", digest.String(), err)
	}
	for _, raw := range []string{
		strings.Repeat("a", 64),
		"SHA256:" + strings.Repeat("a", 64),
		"sha256:" + strings.Repeat("A", 64),
		"sha256:" + strings.Repeat("a", 63),
	} {
		_, err := ParseSHA256Digest(raw)
		if !errors.Is(err, ErrInvalidSHA256Digest) {
			t.Fatalf("ParseSHA256Digest(%q) error = %v, want ErrInvalidSHA256Digest", raw, err)
		}
	}
}

func TestFirmwareVersionIsOpaqueButBounded(t *testing.T) {
	version, err := ParseFirmwareVersion("v2.3.0-build-α")
	if err != nil || version.String() != "v2.3.0-build-α" {
		t.Fatalf("ParseFirmwareVersion() = %q, %v", version.String(), err)
	}
	for _, raw := range []string{"", "v2\n3", string([]byte{0xff}), strings.Repeat("v", maxFirmwareVersionLength+1)} {
		_, err := ParseFirmwareVersion(raw)
		if !errors.Is(err, ErrInvalidFirmwareVersion) {
			t.Fatalf("ParseFirmwareVersion(%q) error = %v, want ErrInvalidFirmwareVersion", raw, err)
		}
	}
}

func TestRemoteTaskURIIsNormalizedWithoutNetworkResolution(t *testing.T) {
	uri, err := ParseRemoteTaskURI("HTTPS://BMC.EXAMPLE/redfish/v1/TaskService/Tasks/42")
	if err != nil {
		t.Fatalf("ParseRemoteTaskURI() error = %v", err)
	}
	if uri.String() != "https://bmc.example/redfish/v1/TaskService/Tasks/42" {
		t.Fatalf("normalized task URI = %q", uri.String())
	}

	for _, raw := range []string{
		"/redfish/v1/TaskService/Tasks/42",
		"ftp://bmc.example/task/42",
		"https://user:password@bmc.example/task/42",
		"https://bmc.example/task/42#fragment",
		"https://bmc.example/task/42?verbose=true",
		"https://bmc.example/",
	} {
		_, err := ParseRemoteTaskURI(raw)
		if !errors.Is(err, ErrInvalidRemoteTaskURI) {
			t.Fatalf("ParseRemoteTaskURI(%q) error = %v, want ErrInvalidRemoteTaskURI", raw, err)
		}
	}
}

func TestEndpointIdentityIsNormalizedAndOpaque(t *testing.T) {
	endpoint, err := ParseEndpointIdentity("http://192.168.1.10/")
	if err != nil {
		t.Fatalf("ParseEndpointIdentity() error = %v", err)
	}
	if endpoint.String() != "http://192.168.1.10" {
		t.Fatalf("normalized endpoint = %q", endpoint.String())
	}

	for _, raw := range []string{
		"bmc.example",
		"ftp://bmc.example",
		"https://user:password@bmc.example",
		"https://bmc.example#fragment",
		"https://bmc.example?query=true",
		"https://bmc.example/redfish",
	} {
		_, err := ParseEndpointIdentity(raw)
		if !errors.Is(err, ErrInvalidEndpoint) {
			t.Fatalf("ParseEndpointIdentity(%q) error = %v, want ErrInvalidEndpoint", raw, err)
		}
	}
}
