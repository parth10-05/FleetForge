// Package redfish implements the bounded, read-only P02 discovery boundary.
package redfish

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
)

var ErrForbiddenAddress = errors.New("forbidden Redfish address")

// TargetAddress fixes the one origin and service-root prefix a discovery
// client may use. It does not accept device-provided arbitrary URLs.
type TargetAddress struct {
	origin      *url.URL
	serviceRoot string
}

func ParseTargetAddress(rawOrigin, targetKey string) (TargetAddress, error) {
	origin, err := url.Parse(rawOrigin)
	if err != nil || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Path != "" && origin.Path != "/" || !safeKey(targetKey) {
		return TargetAddress{}, ErrForbiddenAddress
	}
	origin.Path, origin.RawPath = "", ""
	origin.Host = strings.ToLower(origin.Host)
	return TargetAddress{origin: origin, serviceRoot: "/virtual-bmcs/" + targetKey + "/redfish/v1/"}, nil
}

func (address TargetAddress) Origin() string { return address.origin.String() }
func (address TargetAddress) ServiceRoot() string {
	return address.origin.String() + address.serviceRoot
}

// ResolveResource permits only service-root-relative resource paths. It rejects
// a changed origin, queries, fragments, credentials, and traversal before dial.
func (address TargetAddress) ResolveResource(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrForbiddenAddress
	}
	if (parsed.IsAbs() || parsed.Host != "") && !sameOrigin(parsed, address.origin) {
		return "", ErrForbiddenAddress
	}
	path := parsed.Path
	if !parsed.IsAbs() && !strings.HasPrefix(path, address.serviceRoot) {
		path = strings.TrimPrefix(path, "/")
		path = address.serviceRoot + path
	}
	if !strings.HasPrefix(path, address.serviceRoot) || strings.Contains(path, "..") || strings.ContainsAny(path, "\\\x00") {
		return "", ErrForbiddenAddress
	}
	copy := *address.origin
	copy.Path = path
	return copy.String(), nil
}

func ValidateDialIP(raw string, allow []netip.Prefix) error {
	address, err := netip.ParseAddr(raw)
	if err != nil || !address.IsValid() || address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsUnspecified() || address.IsMulticast() {
		return ErrForbiddenAddress
	}
	for _, prefix := range allow {
		if prefix.Contains(address) {
			return nil
		}
	}
	return ErrForbiddenAddress
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}
func safeKey(key string) bool {
	if key == "" || len(key) > 128 {
		return false
	}
	for _, c := range key {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}
func IsForbidden(err error) bool { return errors.Is(err, ErrForbiddenAddress) }
