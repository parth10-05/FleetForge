# Supported resource matrix

Status: **blocked pending the selected QEMU/OpenBMC capability fixture and an
accepted update contract.** This matrix is deliberately not a guessed Redfish
implementation contract.

| Surface | Standard source/version | Pinned QEMU fixture field | Synthetic contract | Status |
| --- | --- | --- | --- | --- |
| Service Root | Pending recorded DMTF release | Pending | No route until fixture comparison | Blocked |
| Manager, System, Chassis identity | Pending recorded DMTF release | Pending | No properties frozen | Blocked |
| UpdateService discovery/action | Pending recorded DMTF release | Pending route, media type, and apply behavior | No request accepted | Blocked |
| TaskService | Pending recorded DMTF release | Pending task representation/lifecycle | No task representation frozen | Blocked |
| Software/Firmware Inventory | Pending recorded DMTF release | Pending inventory path/version fields | No inventory fields frozen | Blocked |
| EventService | Pending recorded DMTF release | Pending subscription/callback behavior | Fault catalog only; no HTTP contract | Blocked |
| Session/auth subset | Pending recorded DMTF release | Pending trust/auth observation | No auth mode frozen | Blocked |

Before any Redfish handler is implemented, each row must identify:

1. the exact DMTF schema/release used for validation;
2. the separately observed schema/version advertised by the pinned image;
3. one redacted fixture field or response reference;
4. whether the field is standard, QEMU/OpenBMC-specific, or simulator-only;
5. the accepted ADR-039/ADR-032 evidence reference.

Using the latest DMTF release as a substitute for a missing observed target is
forbidden. Unsupported resources, actions, methods, media types, and query
options must remain non-mutating.
