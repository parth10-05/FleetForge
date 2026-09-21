# ADR 032: Supported update contract

## Status

Proposed pending QEMU/OpenBMC capability and recovery evidence.

## Decision

FleetForge v1 will support exactly one simulator-backed update contract. The
contract must identify the selected Redfish service root, update endpoint,
artifact-transfer method, task lifecycle representation, inventory evidence,
reboot behavior, authentication procedure, and recovery procedure.

No API or worker implementation may infer support from vendor names, generic
Redfish discovery, or a successful upload alone. The update path is accepted
only after the selected QEMU/OpenBMC image demonstrates a documented,
reversible test-image update and recovery result.

## Stop condition

If the selected image cannot safely demonstrate this contract, the v1 update
scope stops before Phase 04 consumes it. The contract must be revised through
a new ADR rather than relaxed in application code.

## Required evidence

- image source, checksum, license, and provenance;
- supported Redfish version and trust/credential procedure;
- redacted service, task, and inventory observations;
- upload/apply/reboot timeline for a reversible test image; and
- failure and recovery record.
