package value

import (
	"strings"
	"testing"
	"time"
)

func FuzzBoundedValueParsers(f *testing.F) {
	for _, seed := range []string{"", "not-a-url", "https://bmc.example/redfish/v1/TaskService/Tasks/1", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "00000000-0000-4000-8000-000000000001", "America/New_York", strings.Repeat("x", 300)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		_, err := ParseTargetID(raw)
		assertBoundedError(t, err)
		_, err = ParseSHA256Digest(raw)
		assertBoundedError(t, err)
		_, err = ParseFirmwareVersion(raw)
		assertBoundedError(t, err)
		_, err = ParseEndpointIdentity(raw)
		assertBoundedError(t, err)
		_, err = ParseRemoteTaskURI(raw)
		assertBoundedError(t, err)
		_, err = ParseTimezoneName(raw, time.LoadLocation)
		assertBoundedError(t, err)
	})
}

func assertBoundedError(t *testing.T, err error) {
	t.Helper()
	if err != nil && len(err.Error()) > 128 {
		t.Fatalf("unbounded parser error length %d", len(err.Error()))
	}
}
