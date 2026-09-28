package artifact

import "errors"

var ErrUnsupportedRequiredSignature = errors.New("required artifact signature format is unsupported")

// VerifyRequiredSignature documents the v1 synthetic policy: unsigned
// artifacts may use the SHA-256 verification path, but any requested
// signature format fails closed until a format/key-verifier is implemented.
func VerifyRequiredSignature(requiredFormat string) error {
	if requiredFormat == "" {
		return nil
	}
	return ErrUnsupportedRequiredSignature
}
