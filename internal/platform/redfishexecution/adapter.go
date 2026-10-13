// Package redfishexecution composes the application execution port with the
// local Redfish and credential boundaries. It keeps target addresses and
// credentials out of durable application contracts.
package redfishexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/parth10-05/fleetforge/internal/application/execution"
	"github.com/parth10-05/fleetforge/internal/platform/syntheticcredentials"
	"github.com/parth10-05/fleetforge/internal/redfish"
)

type Target struct {
	Origin, TargetKey, CredentialReference string
}

func (adapter Adapter) Precheck(ctx context.Context, request execution.PrecheckRequest) (execution.PrecheckEvidence, error) {
	if request.TargetID == "" || request.ExpectedVersion == "" {
		return execution.PrecheckEvidence{}, fmt.Errorf("invalid execution precheck request")
	}
	document, _, err := adapter.getDocument(ctx, request.TargetID, "UpdateService/FirmwareInventory/BMC")
	if err != nil {
		return execution.PrecheckEvidence{}, err
	}
	var inventory struct {
		Version    string `json:"Version"`
		Updateable bool   `json:"Updateable"`
		Status     struct {
			Health string `json:"Health"`
		} `json:"Status"`
	}
	if err := json.Unmarshal(document, &inventory); err != nil || inventory.Version == "" {
		return execution.PrecheckEvidence{}, fmt.Errorf("decode firmware inventory: %w", err)
	}
	return execution.PrecheckEvidence{CurrentVersion: inventory.Version, Capable: inventory.Updateable, Healthy: strings.EqualFold(inventory.Status.Health, "OK"), Compatible: inventory.Version != request.ExpectedVersion, EvidenceFingerprint: fingerprint(document), ObservedAt: adapter.now()}, nil
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

func (adapter Adapter) ObserveTask(ctx context.Context, request execution.TaskObservationRequest) (execution.Observation, error) {
	if request.Attempt.RemoteTaskReference == "" {
		return execution.Observation{}, fmt.Errorf("task reference is unavailable")
	}
	document, received, err := adapter.getDocument(ctx, request.Attempt.TargetID, request.Attempt.RemoteTaskReference)
	if err != nil {
		var discoveryError redfish.DiscoveryError
		if errors.As(err, &discoveryError) && discoveryError.Kind == redfish.ErrorNotFound {
			missing := []byte(`{"TaskState":"Missing","Reason":"not_found"}`)
			return adapter.observation(execution.ObservationTask, request.Attempt, request.Attempt.RemoteTaskReference, missing, adapter.now(), false), nil
		}
		return execution.Observation{}, err
	}
	var task struct {
		State string `json:"TaskState"`
	}
	if err := json.Unmarshal(document, &task); err != nil || task.State == "" {
		return execution.Observation{}, fmt.Errorf("decode Redfish task: %w", err)
	}
	return adapter.observation(execution.ObservationTask, request.Attempt, request.Attempt.RemoteTaskReference, document, received, strings.EqualFold(task.State, "Exception") || strings.EqualFold(task.State, "Killed")), nil
}

func (adapter Adapter) ObserveInventory(ctx context.Context, request execution.InventoryObservationRequest) (execution.Observation, error) {
	const resource = "UpdateService/FirmwareInventory/BMC"
	document, received, err := adapter.getDocument(ctx, request.Attempt.TargetID, resource)
	if err != nil {
		return execution.Observation{}, err
	}
	var inventory struct {
		Version string `json:"Version"`
	}
	if err := json.Unmarshal(document, &inventory); err != nil || inventory.Version == "" {
		return execution.Observation{}, fmt.Errorf("decode firmware inventory: %w", err)
	}
	return adapter.observation(execution.ObservationInventory, request.Attempt, resource, document, received, inventory.Version != request.Attempt.ExpectedVersion), nil
}

// ObserveReboot proves only that the target's manager endpoint is reachable
// again. Version and health are separately verified by their own later work
// classes; reachability alone is never success evidence.
func (adapter Adapter) ObserveReboot(ctx context.Context, request execution.RebootObservationRequest) (execution.Observation, error) {
	const resource = "Managers/1"
	document, received, err := adapter.getDocument(ctx, request.Attempt.TargetID, resource)
	if err != nil {
		return execution.Observation{}, err
	}
	var manager map[string]any
	if err := json.Unmarshal(document, &manager); err != nil {
		return execution.Observation{}, fmt.Errorf("decode Redfish reboot reachability: %w", err)
	}
	if manager == nil {
		return execution.Observation{}, fmt.Errorf("decode Redfish reboot reachability: empty document")
	}
	return adapter.observation(execution.ObservationReboot, request.Attempt, resource, document, received, false), nil
}

func (adapter Adapter) ObserveHealth(ctx context.Context, request execution.HealthObservationRequest) (execution.Observation, error) {
	const resource = "Managers/1"
	document, received, err := adapter.getDocument(ctx, request.Attempt.TargetID, resource)
	if err != nil {
		return execution.Observation{}, err
	}
	var status struct {
		Status struct {
			Health string `json:"Health"`
		} `json:"Status"`
	}
	if err := json.Unmarshal(document, &status); err != nil || status.Status.Health == "" {
		return execution.Observation{}, fmt.Errorf("decode Redfish health: %w", err)
	}
	return adapter.observation(execution.ObservationHealth, request.Attempt, resource, document, received, !strings.EqualFold(status.Status.Health, "OK")), nil
}

func (adapter Adapter) getDocument(ctx context.Context, targetID, resource string) ([]byte, time.Time, error) {
	if adapter.Loader == nil || adapter.Client == nil || adapter.Resolver == nil || targetID == "" {
		return nil, time.Time{}, fmt.Errorf("invalid Redfish observation adapter")
	}
	target, err := adapter.Loader.LoadExecutionTarget(ctx, targetID)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("load execution target: %w", err)
	}
	address, err := redfish.ParseTargetAddress(target.Origin, target.TargetKey)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("parse execution target: %w", err)
	}
	credential, err := adapter.Resolver.Resolve(target.CredentialReference)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("resolve execution credential: %w", err)
	}
	// Persisted task references are device-local (/redfish/v1/...) so they
	// cannot smuggle another target origin into a later observation. Convert
	// only that known prefix to the target-scoped relative resource expected by
	// the Redfish address boundary.
	resource = strings.TrimPrefix(resource, "/redfish/v1/")
	document, err := adapter.Client.GetDocumentAuthenticated(ctx, address, resource, credential)
	if err != nil {
		return nil, time.Time{}, err
	}
	return document, adapter.now(), nil
}

func (adapter Adapter) observation(kind execution.ObservationKind, attempt execution.AttemptSnapshot, resource string, document []byte, received time.Time, contradicts bool) execution.Observation {
	normalized, err := normalizeObservation(kind, document)
	if err != nil {
		// Callers have already validated the narrow fields they consume. An
		// empty projection is rejected by the application boundary, preserving
		// fail-closed behavior without retaining the raw target response.
		normalized = nil
	}
	return execution.Observation{Kind: kind, TargetID: attempt.TargetID, TargetUpdateID: attempt.TargetUpdateID, AttemptID: attempt.AttemptID, ResourcePath: resource, EvidenceFingerprint: fingerprint(document), Data: normalized, ObservedAt: received, ReceivedAt: received, Fresh: true, ContradictsExpectedState: contradicts}
}

// normalizeObservation keeps only the fields used by the execution reducer.
// Raw Redfish documents can contain vendor extensions and high-cardinality
// metadata, so they must not cross the durable evidence boundary.
func normalizeObservation(kind execution.ObservationKind, document []byte) ([]byte, error) {
	var source map[string]json.RawMessage
	if err := json.Unmarshal(document, &source); err != nil {
		return nil, err
	}
	projection := map[string]string{}
	switch kind {
	case execution.ObservationTask:
		var taskState string
		if err := json.Unmarshal(source["TaskState"], &taskState); err != nil || taskState == "" {
			return nil, fmt.Errorf("missing task state")
		}
		projection["TaskState"] = taskState
		if raw, ok := source["Reason"]; ok {
			var reason string
			if json.Unmarshal(raw, &reason) == nil && reason != "" {
				projection["Reason"] = reason
			}
		}
	case execution.ObservationInventory:
		var version string
		if err := json.Unmarshal(source["Version"], &version); err != nil || version == "" {
			return nil, fmt.Errorf("missing inventory version")
		}
		projection["Version"] = version
	case execution.ObservationHealth, execution.ObservationReboot:
		var status struct {
			Status struct{ Health, State string } `json:"Status"`
		}
		if err := json.Unmarshal(document, &status); err != nil {
			return nil, err
		}
		if status.Status.Health != "" {
			projection["Health"] = status.Status.Health
		}
		if status.Status.State != "" {
			projection["State"] = status.Status.State
		}
		if len(projection) == 0 {
			return nil, fmt.Errorf("missing manager status")
		}
	default:
		return nil, fmt.Errorf("unknown observation kind")
	}
	return json.Marshal(projection)
}

func fingerprint(document []byte) string {
	digest := sha256.Sum256(document)
	return "sha256:" + hex.EncodeToString(digest[:])
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
	case redfish.FirmwareProvenPrewriteFailure:
		return execution.SubmitProvenPrewriteFailure
	case redfish.FirmwarePossiblyAccepted:
		return execution.SubmitPossiblyAccepted
	default:
		return execution.SubmitUnsafeResponse
	}
}
