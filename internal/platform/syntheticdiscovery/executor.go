// Package syntheticdiscovery composes the local synthetic credential resolver
// with FleetForge's bounded, read-only Redfish client.
package syntheticdiscovery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	app "github.com/parth10-05/fleetforge/internal/application/inventory"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
	"github.com/parth10-05/fleetforge/internal/redfish"
)

// ServiceClient is deliberately limited to authenticated service-root
// discovery; its shape prevents this package from submitting firmware.
type ServiceClient interface {
	DiscoverServiceAuthenticated(context.Context, redfish.TargetAddress, redfish.RequestAuthenticator) (redfish.ServiceCapabilities, error)
}

type Executor struct {
	Loader           app.DiscoveryTargetLoader
	Client           ServiceClient
	Resolver         *syntheticcredentials.Resolver
	Now              func() time.Time
	NewObservationID func() (string, error)
}

func (executor Executor) Discover(ctx context.Context, work app.DiscoveryWork) (app.DiscoveryResult, error) {
	if executor.Loader == nil || executor.Client == nil || executor.Resolver == nil || work.ID == "" || work.TargetID == "" || work.ExpectedRevision < 0 {
		return app.DiscoveryResult{}, fmt.Errorf("invalid synthetic discovery executor")
	}
	target, err := executor.Loader.LoadDiscoveryTarget(ctx, work)
	if err != nil {
		return executor.failureResult("configuration")
	}
	address, err := redfish.ParseTargetAddress(target.Origin, target.TargetKey)
	if err != nil {
		return executor.failureResult("configuration")
	}
	credential, err := executor.Resolver.Resolve(target.CredentialReference)
	if err != nil {
		return executor.failureResult("credential")
	}
	// Target loading above is a single read; no DB transaction or lock remains
	// while this bounded transport call uses the ephemeral credential.
	capabilities, err := executor.Client.DiscoverServiceAuthenticated(ctx, address, credential)
	if err != nil {
		var discoveryError redfish.DiscoveryError
		if errors.As(err, &discoveryError) && discoveryError.Kind.Retryable() {
			return app.DiscoveryResult{}, err
		}
		kind := "unavailable"
		if errors.As(err, &discoveryError) {
			kind = string(discoveryError.Kind)
		}
		return executor.failureResult(kind)
	}
	normalized, err := json.Marshal(struct {
		RedfishVersion   string `json:"redfish_version"`
		SystemsURI       string `json:"systems_uri"`
		ManagersURI      string `json:"managers_uri"`
		UpdateServiceURI string `json:"update_service_uri"`
	}{capabilities.RedfishVersion, capabilities.SystemsURI, capabilities.ManagersURI, capabilities.UpdateServiceURI})
	if err != nil {
		return app.DiscoveryResult{}, fmt.Errorf("encode normalized discovery result: %w", err)
	}
	return executor.result(normalized, true)
}

func (executor Executor) failureResult(kind string) (app.DiscoveryResult, error) {
	normalized, err := json.Marshal(struct {
		ErrorKind string `json:"error_kind"`
	}{kind})
	if err != nil {
		return app.DiscoveryResult{}, err
	}
	return executor.result(normalized, false)
}

func (executor Executor) result(normalized []byte, success bool) (app.DiscoveryResult, error) {
	now := time.Now().UTC()
	if executor.Now != nil {
		now = executor.Now().UTC()
	}
	newID := executor.NewObservationID
	if newID == nil {
		newID = randomUUID
	}
	id, err := newID()
	if err != nil {
		return app.DiscoveryResult{}, fmt.Errorf("create discovery observation id: %w", err)
	}
	return app.DiscoveryResult{ObservationID: id, ObservedAt: now, Normalized: normalized, Success: success}, nil
}

func randomUUID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	hexadecimal := hex.EncodeToString(bytes)
	return hexadecimal[0:8] + "-" + hexadecimal[8:12] + "-" + hexadecimal[12:16] + "-" + hexadecimal[16:20] + "-" + hexadecimal[20:32], nil
}
