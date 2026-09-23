# Internal package boundaries

FleetForge is a modular monolith with independently deployed process roles.
The dependency direction is deliberately inward:

```text
delivery -> application -> domain
platform, persistence, redfish, and observability support or implement inward-facing ports
cmd/* composes dependencies only
```

`internal/domain` must remain pure. It may not import HTTP, SQL, cloud,
Redfish transport, or telemetry SDK packages. `cmd/*` packages perform process
wiring only and may not contain workflow rules. HTTP handlers expose transport
semantics only; they do not make device or persistence calls directly.

Phase 01 creates only bootstrap, configuration, health, lifecycle, and build
metadata behavior. Database, queue, cloud, Redfish, and campaign code belongs
to later approved phases.

The deterministic simulator is a separate test instrument. Its future
`delivery/redfishsim` and `delivery/simulatoradmin` packages may call only an
`application/simulator` boundary; simulator core, scenario, journal, and
clock packages must not import FleetForge persistence or production domain
packages. Platform adapters may implement simulator application ports. A
simulator route is not added until the pinned QEMU/OpenBMC capability evidence
freezes its contract.
