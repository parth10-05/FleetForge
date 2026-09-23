# Scenario and fault catalog

ADR-014 prepares strict JSON scenarios, logical time, deterministic IDs, and
scheduled simulator faults. It does not define a Redfish update request.

## Supported simulator fault identifiers

| ID | Intended synthetic behavior | Status |
| --- | --- | --- |
| RF-F01 | Duplicate event or response | Planned |
| RF-F02 | Late or out-of-order event | Planned |
| RF-F03 | Dropped event | Planned |
| RF-F04 | Controlled `429` observation response | Planned |
| RF-F05 | Controlled `503` observation response | Planned |
| RF-F06 | Connection failure before request-body acceptance | Planned |
| RF-F07 | Disconnect after update acceptance | Planned |
| RF-F08 | Task disappearance after reboot | Planned |
| RF-F10 | Task complete with unchanged version | Planned |
| RF-F11 | Version updated with failed health | Planned |

The v1 scenario decoder must reject RF-F09, RF-F12, RF-F13, RF-F14, and
RF-F15 as unsupported; they are not partially implemented. Each supported
fault will later declare scope, deterministic trigger, bounded consumption,
and priority without logging sensitive request data.
