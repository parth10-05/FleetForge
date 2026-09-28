package artifact

import (
	"errors"
	"testing"
)

func TestRequiredSignatureFailsClosedUntilFormatIsSupported(t *testing.T) {
	if err := VerifyRequiredSignature(""); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRequiredSignature("cms"); !errors.Is(err, ErrUnsupportedRequiredSignature) {
		t.Fatalf("signature error=%v", err)
	}
}
