// Package inventory owns pure target-inventory invariants.
package inventory

import (
	"errors"
	"fmt"
	"strings"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

const (
	SyntheticContractV1         = "synthetic-redfish-v1"
	maxSyntheticTargetKeyLength = 128
)

var ErrInvalidSyntheticTargetAddress = errors.New("invalid synthetic target address")

// SyntheticTargetAddress identifies exactly one synthetic Redfish service. It
// deliberately accepts an origin and a target key rather than an arbitrary
// caller-controlled path, so registration cannot target simulator admin paths.
type SyntheticTargetAddress struct {
	origin          value.EndpointIdentity
	targetKey       string
	contractVersion string
}

func ParseSyntheticTargetAddress(origin, targetKey, contractVersion string) (SyntheticTargetAddress, error) {
	endpoint, err := value.ParseEndpointIdentity(origin)
	if err != nil || !validTargetKey(targetKey) || contractVersion != SyntheticContractV1 {
		return SyntheticTargetAddress{}, ErrInvalidSyntheticTargetAddress
	}
	return SyntheticTargetAddress{origin: endpoint, targetKey: targetKey, contractVersion: contractVersion}, nil
}

func (address SyntheticTargetAddress) Origin() value.EndpointIdentity { return address.origin }
func (address SyntheticTargetAddress) TargetKey() string              { return address.targetKey }
func (address SyntheticTargetAddress) ContractVersion() string        { return address.contractVersion }

// ServiceRoot is derived only from the validated origin and key. It is not an
// input and callers must still validate device-returned URIs at transport time.
func (address SyntheticTargetAddress) ServiceRoot() string {
	return fmt.Sprintf("%s/virtual-bmcs/%s/redfish/v1/", address.origin.String(), address.targetKey)
}

func (address SyntheticTargetAddress) String() string {
	return address.contractVersion + ":" + address.ServiceRoot()
}

func validTargetKey(raw string) bool {
	if len(raw) == 0 || len(raw) > maxSyntheticTargetKeyLength {
		return false
	}
	for index := range len(raw) {
		character := raw[index]
		if index == 0 {
			if !asciiAlphaNumeric(character) {
				return false
			}
			continue
		}
		if !asciiAlphaNumeric(character) && !strings.ContainsRune(".-_", rune(character)) {
			return false
		}
	}
	return true
}

func asciiAlphaNumeric(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'
}
