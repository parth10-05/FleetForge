// Package redfishexecution composes the application execution port with the
// local Redfish and credential boundaries. It keeps target addresses and
// credentials out of durable application contracts.
package redfishexecution

import (
	"context"
	"io"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/execution"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
	"github.com/parth10-05/fleetforge/internal/redfish"
)

type Target struct {
	Origin, TargetKey, CredentialReference string
}

// TargetLoader performs the one configuration read required before an
// external call. It must not retain a workflow transaction while loading.
type TargetLoader interface {
	LoadExecutionTarget(context.Context, string) (Target, error)
}

type Adapter struct {
	Loader   TargetLoader
	Client   *redfish.Client
	Resolver *syntheticcredentials.Resolver
	Now      func() time.Time
}

// Submit translates an admitted attempt into exactly one target request. Any
// configuration or target-protocol anomaly is unsafe, not a retry signal.
func (adapter Adapter) Submit(ctx context.Context, request execution.SubmitRequest, artifact io.Reader) execution.SubmitResult {
	finished := adapter.now()
	unsafe := func() execution.SubmitResult {
		return execution.SubmitResult{Class: execution.SubmitUnsafeResponse, FinishedAt: finished}
	}
	if adapter.Loader == nil || adapter.Client == nil || adapter.Resolver == nil || artifact == nil || request.Attempt.AttemptID == "" || request.Attempt.TargetID == "" || request.Attempt.ExpectedVersion == "" || request.Attempt.Artifact.SHA256 == "" || request.UpdateActionPath == "" {
		return unsafe()
	}
	target, err := adapter.Loader.LoadExecutionTarget(ctx, request.Attempt.TargetID)
	if err != nil {
		return unsafe()
	}
	address, err := redfish.ParseTargetAddress(target.Origin, target.TargetKey)
	if err != nil {
		return unsafe()
	}
	credential, err := adapter.Resolver.Resolve(target.CredentialReference)
	if err != nil {
		return unsafe()
	}
	result := adapter.Client.SubmitFirmwareAuthenticated(ctx, address, credential, redfish.FirmwareSubmitRequest{
		ActionPath:      request.UpdateActionPath,
		ExpectedVersion: request.Attempt.ExpectedVersion,
		ArtifactSHA256:  request.Attempt.Artifact.SHA256,
		AttemptID:       request.Attempt.AttemptID,
		Body:            artifact,
	})
	return execution.SubmitResult{
		Class:               translateSubmitClass(result.Class),
		RemoteTaskReference: result.RemoteTaskReference,
		ResponseFingerprint: result.ResponseFingerprint,
		FinishedAt:          result.FinishedAt,
	}
}

func (adapter Adapter) now() time.Time {
	if adapter.Now != nil {
		return adapter.Now().UTC()
	}
	return time.Now().UTC()
}

func translateSubmitClass(class redfish.FirmwareSubmitClass) execution.SubmitClass {
	switch class {
	case redfish.FirmwareAccepted:
		return execution.SubmitAccepted
	case redfish.FirmwareExplicitRejection:
		return execution.SubmitExplicitRejection
	case redfish.FirmwarePossiblyAccepted:
		return execution.SubmitPossiblyAccepted
	default:
		return execution.SubmitUnsafeResponse
	}
}
