package value

import (
	"errors"
	"net/url"
	"strings"
)

const maxFirmwareVersionLength = 256

var (
	ErrInvalidSHA256Digest    = errors.New("invalid SHA-256 digest")
	ErrInvalidFirmwareVersion = errors.New("invalid firmware version")
	ErrInvalidRemoteTaskURI   = errors.New("invalid remote task URI")
	ErrInvalidEndpoint        = errors.New("invalid endpoint identity")
)

// SHA256Digest is a canonical algorithm-qualified lowercase SHA-256 digest.
type SHA256Digest struct {
	value string
}

func ParseSHA256Digest(raw string) (SHA256Digest, error) {
	const prefix = "sha256:"
	if !strings.HasPrefix(raw, prefix) || len(raw) != len(prefix)+64 {
		return SHA256Digest{}, ErrInvalidSHA256Digest
	}
	for index := len(prefix); index < len(raw); index++ {
		if !isLowerHex(raw[index]) {
			return SHA256Digest{}, ErrInvalidSHA256Digest
		}
	}
	return SHA256Digest{value: raw}, nil
}

func (digest SHA256Digest) String() string { return digest.value }

func (digest SHA256Digest) IsZero() bool { return digest.value == "" }

// FirmwareVersion is an opaque, bounded version identifier. It deliberately
// does not impose semantic-version ordering on vendor firmware strings.
type FirmwareVersion struct {
	value string
}

func ParseFirmwareVersion(raw string) (FirmwareVersion, error) {
	if !isBoundedVisibleText(raw, maxFirmwareVersionLength) {
		return FirmwareVersion{}, ErrInvalidFirmwareVersion
	}
	return FirmwareVersion{value: raw}, nil
}

func (version FirmwareVersion) String() string { return version.value }

func (version FirmwareVersion) IsZero() bool { return version.value == "" }

// RemoteTaskURI identifies a previously accepted remote operation. Parsing is
// syntax-only: it never resolves DNS or reaches a network.
type RemoteTaskURI struct {
	value string
}

func ParseRemoteTaskURI(raw string) (RemoteTaskURI, error) {
	parsed, err := parseHTTPURI(raw)
	if err != nil || parsed.Path == "" || parsed.Path == "/" || parsed.RawQuery != "" {
		return RemoteTaskURI{}, ErrInvalidRemoteTaskURI
	}
	return RemoteTaskURI{value: parsed.String()}, nil
}

func (uri RemoteTaskURI) String() string { return uri.value }

func (uri RemoteTaskURI) IsZero() bool { return uri.value == "" }

// EndpointIdentity is a normalized HTTP(S) authority. It is opaque and does
// not perform reachability, DNS, IP-range, or SSRF validation.
type EndpointIdentity struct {
	value string
}

func ParseEndpointIdentity(raw string) (EndpointIdentity, error) {
	parsed, err := parseHTTPURI(raw)
	if err != nil || parsed.RawQuery != "" || (parsed.Path != "" && parsed.Path != "/") {
		return EndpointIdentity{}, ErrInvalidEndpoint
	}
	return EndpointIdentity{value: parsed.Scheme + "://" + parsed.Host}, nil
}

func (endpoint EndpointIdentity) String() string { return endpoint.value }

func (endpoint EndpointIdentity) IsZero() bool { return endpoint.value == "" }

func parseHTTPURI(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.Opaque != "" || parsed.User != nil || parsed.Fragment != "" || parsed.Host == "" {
		return nil, ErrInvalidRemoteTaskURI
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, ErrInvalidRemoteTaskURI
	}
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.RawPath != "" {
		return nil, ErrInvalidRemoteTaskURI
	}
	return parsed, nil
}
