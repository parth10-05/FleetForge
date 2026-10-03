// Package syntheticcredentials resolves only local simulator test references.
// It is intentionally not an AWS Secrets Manager substitute; production
// credential provisioning and resolution remain outside the local adapter.
package syntheticcredentials

import (
	"errors"
	"net/http"
	"strings"
)

const referencePrefix = "synthetic-test:"

var (
	ErrInvalidReference  = errors.New("invalid synthetic credential reference")
	ErrUnknownReference  = errors.New("unknown synthetic credential reference")
	ErrInvalidCredential = errors.New("invalid synthetic credential")
)

// Credential intentionally has no String method. It may be applied to an
// outbound request by a transport adapter but must not enter logs, events, or
// persistence records.
type Credential struct {
	username string
	password string
}

// Apply adds the credential to an outbound request without exposing a secret
// value to callers that do not need it.
func (credential Credential) Apply(request *http.Request) {
	request.SetBasicAuth(credential.username, credential.password)
}

// Resolver's keys are opaque names without secret material. Values are held
// in process memory only for the local simulator contract.
type Resolver struct{ credentials map[string]Credential }

func New(credentials map[string]Credential) (*Resolver, error) {
	copy := make(map[string]Credential, len(credentials))
	for name, credential := range credentials {
		if !validName(name) || credential.username == "" || credential.password == "" {
			return nil, ErrInvalidCredential
		}
		copy[name] = credential
	}
	return &Resolver{credentials: copy}, nil
}

func NewCredential(username, password string) (Credential, error) {
	if username == "" || password == "" || strings.ContainsAny(username, "\r\n") || strings.ContainsAny(password, "\r\n") {
		return Credential{}, ErrInvalidCredential
	}
	return Credential{username: username, password: password}, nil
}

func (resolver *Resolver) Resolve(reference string) (Credential, error) {
	name, err := parseReference(reference)
	if err != nil {
		return Credential{}, err
	}
	if resolver == nil {
		return Credential{}, ErrUnknownReference
	}
	credential, exists := resolver.credentials[name]
	if !exists {
		return Credential{}, ErrUnknownReference
	}
	return credential, nil
}

func parseReference(reference string) (string, error) {
	name, ok := strings.CutPrefix(reference, referencePrefix)
	if !ok || !validName(name) {
		return "", ErrInvalidReference
	}
	return name, nil
}

func validName(name string) bool {
	if len(name) == 0 || len(name) > 96 {
		return false
	}
	for _, character := range name {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == '-') {
			return false
		}
	}
	return true
}
