# ADR 032: Supported synthetic target update contract

## Status

Accepted for the FleetForge synthetic target backend.

## Decision

FleetForge v1 supports exactly one synthetic update contract implemented by
the repository-owned synthetic target backend. The contract identifies the selected
Redfish-shaped service root, update endpoint, bounded artifact-transfer method,
task lifecycle, inventory evidence, reboot behavior, authentication procedure,
fault behavior, and recovery procedure.

The backend accepts only synthetic artifacts and never interprets uploaded
bytes as executable firmware. Its independent journal records every received
request and every accepted operation before response-loss injection. Repeated
accepted requests remain repeated visible side effects; the backend does not
deduplicate on FleetForge attempt or command identity.

The supported surface is derived from a pinned DMTF schema release and labels
all FleetForge extensions explicitly. It is a project-owned test contract, not
a vendor, hardware, or full Redfish compatibility statement.

## Stop condition

If an update, task, inventory, reboot, health, or ambiguity behavior is absent
from this ADR and its supported-resource matrix, the implementation stops and
the contract is revised through an ADR. Application code may not infer missing
behavior or weaken the independent acceptance oracle.

## Required evidence

- pinned DMTF release and checksums for the selected schemas;
- versioned canonical service, task, inventory, event, and error fixtures;
- scenario, seed, request-trace, and backend-build identity;
- synthetic upload, task, reboot, inventory, and health timeline;
- pre-accept, post-accept, retry, contradiction, and recovery records; and
- explicit limitations stating that no external device was tested.
