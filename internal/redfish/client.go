package redfish

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

var (
	ErrRedirectDenied       = errors.New("Redfish redirect denied")
	ErrUnexpectedStatus     = errors.New("unexpected Redfish response status")
	ErrUnexpectedMediaType  = errors.New("unexpected Redfish response content type")
	ErrRequestNotAuthorized = errors.New("Redfish request not authorized")
)

// ErrorKind is the stable discovery failure vocabulary exposed to callers.
// It intentionally describes the transport outcome rather than a vendor
// specific error payload.
type ErrorKind string

const (
	ErrorUnauthenticated ErrorKind = "unauthenticated"
	ErrorUnauthorized    ErrorKind = "unauthorized"
	ErrorUnreachable     ErrorKind = "unreachable"
	ErrorRateLimited     ErrorKind = "rate_limited"
	ErrorMalformed       ErrorKind = "malformed"
	ErrorNotFound        ErrorKind = "not_found"
	ErrorUnsupported     ErrorKind = "unsupported"
	ErrorTransient       ErrorKind = "transient_server"
	ErrorTimeout         ErrorKind = "timeout"
)

type DiscoveryError struct {
	Kind ErrorKind
	err  error
}

func (error DiscoveryError) Error() string { return string(error.Kind) + " Redfish discovery failure" }
func (error DiscoveryError) Unwrap() error { return error.err }

// Retryable reports only transport outcomes for which a later read may safely
// produce new evidence. Authentication, authorization, malformed, and
// unsupported responses require an explicit configuration or compatibility
// change; retrying them indefinitely would hide a real safety failure.
func (kind ErrorKind) Retryable() bool {
	return kind == ErrorUnreachable || kind == ErrorRateLimited || kind == ErrorTransient || kind == ErrorTimeout
}

// Client is a read-only Redfish document client. Its narrow API deliberately
// prevents discovery from reaching an update/action endpoint.
type Client struct {
	httpClient *http.Client
	maxBytes   int
	maxDepth   int
	total      time.Duration
}

type ClientConfig struct {
	// HTTPClient is for a controlled test transport. Production callers leave
	// it nil and use the restrictive default transport below.
	HTTPClient   *http.Client
	MaxBytes     int
	MaxDepth     int
	Total        time.Duration
	AllowedCIDRs []netip.Prefix
	ResolveIP    func(context.Context, string) ([]netip.Addr, error)
}

// RequestAuthenticator applies credentials directly to an outbound request.
// Implementations must not expose or log secret values.
type RequestAuthenticator interface{ Apply(*http.Request) }

func NewClient(config ClientConfig) *Client {
	maxBytes := config.MaxBytes
	if maxBytes == 0 {
		maxBytes = DefaultDocumentMaxBytes
	}
	maxDepth := config.MaxDepth
	if maxDepth == 0 {
		maxDepth = DefaultDocumentMaxDepth
	}
	total := config.Total
	if total == 0 {
		total = 15 * time.Second
	}

	client := config.HTTPClient
	if client == nil {
		resolver := config.ResolveIP
		if resolver == nil {
			resolver = func(ctx context.Context, host string) ([]netip.Addr, error) {
				return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			}
		}
		client = &http.Client{Transport: &http.Transport{
			Proxy:                 nil,
			ResponseHeaderTimeout: 5 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			DisableCompression:    true,
			DialContext:           dialApprovedIP(config.AllowedCIDRs, resolver),
		}, Timeout: total}
	} else {
		copy := *client
		client = &copy
	}
	// Redirects can carry credentials to an attacker-controlled address. Never
	// follow one; a device must return a valid service-root-relative URI.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return ErrRedirectDenied }
	return &Client{httpClient: client, maxBytes: maxBytes, maxDepth: maxDepth, total: total}
}

func dialApprovedIP(allow []netip.Prefix, resolve func(context.Context, string) ([]netip.Addr, error)) func(context.Context, string, string) (net.Conn, error) {
	dialer := net.Dialer{}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, ErrForbiddenAddress
		}
		ips, err := resolve(ctx, host)
		if err != nil {
			return nil, ErrForbiddenAddress
		}
		for _, ip := range ips {
			if ValidateDialIP(ip.String(), allow) == nil {
				return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			}
		}
		return nil, ErrForbiddenAddress
	}
}

// GetDocument retrieves and validates one device document. resource must have
// passed TargetAddress's service-root and origin restrictions.
func (client *Client) GetDocument(ctx context.Context, address TargetAddress, resource string) ([]byte, error) {
	return client.getDocument(ctx, address, resource, nil)
}

func (client *Client) GetDocumentAuthenticated(ctx context.Context, address TargetAddress, resource string, authenticator RequestAuthenticator) ([]byte, error) {
	if authenticator == nil {
		return nil, DiscoveryError{Kind: ErrorUnauthenticated, err: ErrRequestNotAuthorized}
	}
	return client.getDocument(ctx, address, resource, authenticator)
}

func (client *Client) getDocument(ctx context.Context, address TargetAddress, resource string, authenticator RequestAuthenticator) ([]byte, error) {
	if client == nil || client.httpClient == nil || client.maxBytes <= 0 || client.maxDepth <= 0 || client.total <= 0 {
		return nil, DiscoveryError{Kind: ErrorMalformed, err: ErrRequestNotAuthorized}
	}
	endpoint, err := address.ResolveResource(resource)
	if err != nil {
		return nil, DiscoveryError{Kind: ErrorMalformed, err: err}
	}
	requestContext, cancel := context.WithTimeout(ctx, client.total)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, DiscoveryError{Kind: ErrorMalformed, err: err}
	}
	request.Header.Set("Accept", "application/json")
	if authenticator != nil {
		authenticator.Apply(request)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, classifyTransportError(err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized {
		return nil, DiscoveryError{Kind: ErrorUnauthenticated, err: ErrUnexpectedStatus}
	}
	if response.StatusCode == http.StatusForbidden {
		return nil, DiscoveryError{Kind: ErrorUnauthorized, err: ErrUnexpectedStatus}
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return nil, DiscoveryError{Kind: ErrorRateLimited, err: ErrUnexpectedStatus}
	}
	if response.StatusCode >= 500 && response.StatusCode <= 599 {
		return nil, DiscoveryError{Kind: ErrorTransient, err: ErrUnexpectedStatus}
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, DiscoveryError{Kind: ErrorNotFound, err: ErrUnexpectedStatus}
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, DiscoveryError{Kind: ErrorUnsupported, err: fmt.Errorf("%w: %d", ErrUnexpectedStatus, response.StatusCode)}
	}
	if response.ContentLength > int64(client.maxBytes) {
		return nil, DiscoveryError{Kind: ErrorMalformed, err: ErrResponseTooLarge}
	}
	if !jsonContentType(response.Header.Get("Content-Type")) {
		return nil, DiscoveryError{Kind: ErrorMalformed, err: ErrUnexpectedMediaType}
	}
	document, err := DecodeDocument(response.Body, client.maxBytes, client.maxDepth)
	if err != nil {
		return nil, DiscoveryError{Kind: ErrorMalformed, err: err}
	}
	return document, nil
}

func jsonContentType(raw string) bool {
	mediaType, _, err := mime.ParseMediaType(raw)
	return err == nil && strings.EqualFold(mediaType, "application/json")
}

func classifyTransportError(err error) error {
	if IsForbidden(err) {
		return DiscoveryError{Kind: ErrorMalformed, err: err}
	}
	if errors.Is(err, ErrRedirectDenied) {
		return DiscoveryError{Kind: ErrorMalformed, err: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return DiscoveryError{Kind: ErrorTimeout, err: err}
	}
	return DiscoveryError{Kind: ErrorUnreachable, err: err}
}
