# ADR-039 — Selected Redfish/OpenBMC firmware update contract

## Status

Proposed pending a pinned QEMU/OpenBMC capability and recovery record.

## Context

FleetForge must not infer a firmware-update route from generic Redfish
discovery, a vendor name, or a successful upload. The simulator and later
adapter require one exact observed contract from the selected image.

## Decision required

Before an update handler or update-specific simulator behavior is implemented,
this record must identify the selected image and the exact service root,
endpoint/action, request shape and content type, artifact-transfer mechanism,
apply-time behavior, task representation, firmware-inventory evidence,
reboot behavior, authentication/trust procedure, failure behavior, and
recovery procedure.

## Evidence required

- immutable image source, checksum, license, provenance, and retrieval date;
- QEMU, OpenBMC, bmcweb, and advertised Redfish schema/version identifiers;
- redacted service-root, update, task, and inventory observations;
- a reversible test-image update with timing, failure, and recovery evidence;
- a redaction review confirming no credentials, session values, firmware bytes,
  signed URLs, or real endpoints are retained.

## Stop condition

If the selected image cannot safely demonstrate a recoverable contract, v1
stops before route-dependent simulator or adapter work. A synthetic simulator
must not fill the missing evidence gap.
