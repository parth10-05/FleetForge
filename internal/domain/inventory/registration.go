package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"unicode"

	"github.com/parth10-05/fleetforge/internal/domain/value"
)

const SyntheticTransportPolicyV1 = "synthetic-allowlisted-v1"

var ErrInvalidRegistration = errors.New("invalid target registration")

// Registration is the complete semantic input for a simulator target.
// It stores an opaque credential reference, never a credential value.
type Registration struct {
	externalID          string
	address             SyntheticTargetAddress
	credentialReference string
	transportPolicy     string
	labels              value.FailureDomainLabels
}

func NewRegistration(externalID, origin, targetKey, contractVersion, credentialReference, transportPolicy string, labels map[string]string) (Registration, error) {
	if !validExternalID(externalID) || !validCredentialReference(credentialReference) || transportPolicy != SyntheticTransportPolicyV1 {
		return Registration{}, ErrInvalidRegistration
	}
	address, err := ParseSyntheticTargetAddress(origin, targetKey, contractVersion)
	if err != nil {
		return Registration{}, ErrInvalidRegistration
	}
	parsedLabels, err := value.NewFailureDomainLabels(labels)
	if err != nil {
		return Registration{}, ErrInvalidRegistration
	}
	return Registration{externalID: externalID, address: address, credentialReference: credentialReference, transportPolicy: transportPolicy, labels: parsedLabels}, nil
}

func (registration Registration) ExternalID() string              { return registration.externalID }
func (registration Registration) Address() SyntheticTargetAddress { return registration.address }
func (registration Registration) CredentialReference() string {
	return registration.credentialReference
}
func (registration Registration) TransportPolicy() string           { return registration.transportPolicy }
func (registration Registration) Labels() value.FailureDomainLabels { return registration.labels }

// SemanticHash is stable across map iteration and is the idempotency identity
// for a registration request. It contains the opaque reference name but never
// a credential value.
func (registration Registration) SemanticHash() value.SHA256Digest {
	parts := []string{
		registration.externalID,
		registration.address.Origin().String(),
		registration.address.TargetKey(),
		registration.address.ContractVersion(),
		registration.credentialReference,
		registration.transportPolicy,
	}
	labels := registration.labels.AsMap()
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key, labels[key])
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	digest, _ := value.ParseSHA256Digest("sha256:" + hex.EncodeToString(sum[:]))
	return digest
}

func validExternalID(raw string) bool {
	if len(raw) == 0 || len(raw) > 256 {
		return false
	}
	for _, character := range raw {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validCredentialReference(raw string) bool {
	name, found := strings.CutPrefix(raw, "synthetic-test:")
	if !found || len(name) == 0 || len(name) > 96 {
		return false
	}
	for _, character := range name {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == '-') {
			return false
		}
	}
	return true
}
