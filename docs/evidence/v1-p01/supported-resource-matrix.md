# Supported resource matrix

Status: **synthetic contract refreeze approved; exact selected fields and
fixtures must be completed before route implementation.** This matrix defines
the repository-owned target boundary and is not an external compatibility
claim.

| Surface | Pinned DMTF source/version | Selected fields | Simulator extension | Status |
| --- | --- | --- | --- | --- |
| Service Root | Pending Run 01 pin | Links to supported collections/services | Target prefix for multi-target process | Refreeze required |
| Manager, System, Chassis identity | Pending Run 01 pin | Stable identity, reachability, status, health | Deterministic synthetic model/family values | Refreeze required |
| UpdateService discovery/action | Pending Run 01 pin | Service state, inventory link, `HttpPushUri` | Digest/version/attempt headers for synthetic artifacts | Refreeze required |
| TaskService | Pending Run 01 pin | Task state/status/percent/messages and collection | Deterministic operation correlation fingerprint | Refreeze required |
| Software/Firmware Inventory | Pending Run 01 pin | Current version, updateable flag, state, health | Scenario-declared expected synthetic version | Refreeze required |
| EventService | Pending Run 01 pin | Subscription and event payload fields consumed by adapter | Deterministic loss/duplicate/reorder controls | Refreeze required |
| Authentication | HTTP Basic boundary | `401` challenge and authenticated requests | Local credential injection; separate admin credential | Refreeze required |

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
