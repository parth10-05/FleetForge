# Supported resource matrix

Status: **frozen before route implementation.** This matrix defines
the repository-owned target boundary and is not an external compatibility
claim.

| Surface | Pinned DMTF source/version | Selected fields | Simulator extension | Status |
| --- | --- | --- | --- | --- |
| Service Root | ServiceRoot.v1_9_4 | Links to supported collections/services | Target prefix for multi-target process | `service-root.json` |
| Manager, System, Chassis identity | Manager.v1_9_8, ComputerSystem.v1_9_9, Chassis.v1_9_9 | Stable identity, reachability, status, health | Deterministic synthetic model/family values | `manager.json`, `system.json`, `chassis-1.json` |
| UpdateService discovery/action | UpdateService.v1_9_8 | Service state, inventory link, `HttpPushUri` | Digest/version/attempt headers for synthetic artifacts | `update-service.json`, `update-accepted.json` |
| TaskService | TaskService.v1_3_0, Task.v1_7_4 | Task state/status/percent/messages and collection | Deterministic operation correlation fingerprint | `task-service.json`, `task.json` |
| Software/Firmware Inventory | SoftwareInventory.v1_9_2 | Current version, updateable flag, state, health | Scenario-declared expected synthetic version | `firmware-inventory.json`, `firmware-item.json` |
| EventService | EventService.v1_9_4, EventDestination.v1_9_9 | Subscription and event payload fields consumed by adapter | Deterministic loss/duplicate/reorder controls | `event-service.json`, `subscription-created.json` |
| Authentication | HTTP Basic boundary | `401` challenge and authenticated requests | Local credential injection; separate admin credential | `error-unauthorized.json` |

Before any Redfish handler is implemented, each row must identify:

1. the exact DMTF schema/release and downloaded-schema checksum;
2. every selected field and canonical fixture reference;
3. whether each value is selected-standard, simulator-extension, or admin-only;
4. limits, unsupported methods/options, and non-mutating error behavior;
5. the accepted ADR-032/038/039 evidence reference.

Mutable “latest” references are forbidden after the matrix is frozen.
Unsupported resources, actions, methods, media types, and query options remain
non-mutating. Schema validation establishes selected-field discipline, not
complete Redfish conformance.
