package redfish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FirmwareSubmitClass describes only what the bounded HTTP exchange proves.
// In particular, a transport failure after a request is constructed is never
// interpreted as a pre-write failure: a target might have accepted it.
type FirmwareSubmitClass string

const (
	FirmwareAccepted          FirmwareSubmitClass = "ACCEPTED"
	FirmwareExplicitRejection FirmwareSubmitClass = "EXPLICIT_REJECTION"
	FirmwarePossiblyAccepted  FirmwareSubmitClass = "POSSIBLY_ACCEPTED"
	FirmwareUnsafeResponse    FirmwareSubmitClass = "UNSAFE_RESPONSE"
)

type FirmwareSubmitRequest struct {
	ActionPath, ExpectedVersion, ArtifactSHA256, AttemptID string
	Body                                                   io.Reader
}

type FirmwareSubmitResult struct {
	Class                                    FirmwareSubmitClass
	RemoteTaskReference, ResponseFingerprint string
	FinishedAt                               time.Time
}

// SubmitFirmwareAuthenticated performs exactly one HTTP push. It does not
// retry internally; callers must persist the returned outcome before deciding
// whether independent reconciliation is required.
func (client *Client) SubmitFirmwareAuthenticated(ctx context.Context, address TargetAddress, authenticator RequestAuthenticator, request FirmwareSubmitRequest) FirmwareSubmitResult {
	finished := time.Now().UTC()
	unsafe := func() FirmwareSubmitResult {
		return FirmwareSubmitResult{Class: FirmwareUnsafeResponse, FinishedAt: finished}
	}
	if client == nil || client.httpClient == nil || authenticator == nil || request.Body == nil || request.ExpectedVersion == "" || request.AttemptID == "" || !validFirmwareSHA256(request.ArtifactSHA256) {
		return unsafe()
	}
	endpoint, err := address.ResolveResource(request.ActionPath)
	if err != nil {
		return unsafe()
	}
	requestContext, cancel := context.WithTimeout(ctx, client.total)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodPost, endpoint, request.Body)
	if err != nil {
		return unsafe()
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", "application/octet-stream")
	httpRequest.Header.Set("X-FleetForge-Expected-Version", request.ExpectedVersion)
	httpRequest.Header.Set("X-FleetForge-Artifact-SHA256", strings.TrimPrefix(request.ArtifactSHA256, "sha256:"))
	httpRequest.Header.Set("X-FleetForge-Attempt", request.AttemptID)
	authenticator.Apply(httpRequest)

	response, err := client.httpClient.Do(httpRequest)
	finished = time.Now().UTC()
	if err != nil {
		return FirmwareSubmitResult{Class: FirmwarePossiblyAccepted, FinishedAt: finished}
	}
	defer response.Body.Close()
	fingerprint, readOK := firmwareResponseFingerprint(response, client.maxBytes)
	if response.StatusCode == http.StatusAccepted {
		reference, err := localTaskReference(address, response.Header.Get("Location"))
		if err == nil && readOK {
			return FirmwareSubmitResult{Class: FirmwareAccepted, RemoteTaskReference: reference, ResponseFingerprint: fingerprint, FinishedAt: finished}
		}
		return FirmwareSubmitResult{Class: FirmwareUnsafeResponse, ResponseFingerprint: fingerprint, FinishedAt: finished}
	}
	if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != http.StatusRequestTimeout && response.StatusCode != http.StatusTooManyRequests {
		return FirmwareSubmitResult{Class: FirmwareExplicitRejection, ResponseFingerprint: fingerprint, FinishedAt: finished}
	}
	return FirmwareSubmitResult{Class: FirmwareUnsafeResponse, ResponseFingerprint: fingerprint, FinishedAt: finished}
}

func validFirmwareSHA256(value string) bool {
	value = strings.TrimPrefix(value, "sha256:")
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func firmwareResponseFingerprint(response *http.Response, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		return "", false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(maxBytes)+1))
	if err != nil || len(body) > maxBytes {
		return "", false
	}
	material := fmt.Sprintf("%d\n%s\n%s\n", response.StatusCode, response.Header.Get("Location"), body)
	digest := sha256.Sum256([]byte(material))
	return "sha256:" + hex.EncodeToString(digest[:]), true
}

func localTaskReference(address TargetAddress, location string) (string, error) {
	resolved, err := address.ResolveResource(location)
	if err != nil || !strings.HasPrefix(resolved, address.origin.String()+address.serviceRoot) {
		return "", ErrForbiddenAddress
	}
	path := strings.TrimPrefix(resolved, address.origin.String()+"/virtual-bmcs/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[1], "redfish/v1/TaskService/Tasks/") {
		return "", ErrForbiddenAddress
	}
	return "/" + parts[1], nil
}
