# Deferred simulator and Phase 04 work

The following claims and work remain deferred until the QEMU/OpenBMC capability
spike supplies a redacted fixture and ADR-013/ADR-032 are accepted for one
exact update route:

- Redfish listener routes, resource properties, schema contracts, update media
  type, task shape, firmware inventory evidence, and authentication subset.
- Any accepted simulated firmware update, reboot lifecycle, callback protocol,
  or update-specific fault injection.
- QEMU image provenance, reversible update, recovery, and compatibility proof.
- EC2 deployment/start/stop, AWS resource provisioning, or cloud smoke tests.
- RF-F09, RF-F12, RF-F13, RF-F14, RF-F15, full 1,000-target measurement, and
  Phase 04 load proof.

Preparation may proceed only where it cannot invent the missing route. A
simulator-only test cannot establish vendor, QEMU, or physical-device behavior.
