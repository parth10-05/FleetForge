package postgres

import "testing"

func TestResponseReferenceRoundTrip(t *testing.T) {
	reference := ResponseReference{Operation: "register-target", ResourceID: "00000000-0000-0000-0000-000000000001", Revision: 0, CommandID: "00000000-0000-0000-0000-000000000002"}
	encoded, err := reference.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeResponseReference(encoded)
	if err != nil || decoded != reference {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}
	for _, value := range []string{"", "v1ref:register-target:bad:0:bad", "v1ref:register-target:00000000-0000-0000-0000-000000000001:00:00000000-0000-0000-0000-000000000002"} {
		if _, err := DecodeResponseReference(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
