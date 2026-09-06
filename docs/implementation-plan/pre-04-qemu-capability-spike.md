# Pre Phase 04 QEMU OpenBMC Capability Spike

## Outcome

Run one bounded, on-demand investigation against the exact prebuilt QEMU/OpenBMC image intended for FleetForge. Freeze only the protocol and artifact-delivery behavior that the simulator and Redfish adapter may depend on. This spike is not the final portfolio integration proof.

## Entry criteria

- Phase 03 persistence and the Phase 13 early cloud bootstrap pass.
- The EC2 VM, network, firewall, dedicated credentials, TLS trust, snapshot/rebuild path, cost cap, and teardown command are ready.
- Candidate image source, commit/version, checksum, license/provenance, and supported QEMU machine are recorded.
- No production target route or credential is reachable from the spike environment.

## Required questions

1. Which UpdateService mechanism is actually present: raw `HttpPushUri`, `MultipartHttpPushUri`, `SimpleUpdate`, or another explicitly supported action?
2. Does the BMC accept an HTTPS object URI, or must FleetForge stream the exact S3 version ID through the worker?
3. Which apply-time modes are supported, and does activation require an explicit BMC/host reset?
4. What request format, image type, content type, maximum size, timeout, and authentication/session behavior are observed?
5. Is a task URI returned? Does it survive reboot? Which TaskService, SoftwareInventory, activation, version, and health evidence is available?
6. Which events are supported and how are they authenticated/correlated?
7. What is the safe restore/rebuild procedure after a failed update?

## Micro tasks

- [ ] **QSP-T01** Pin the image, source, checksum, machine, bmcweb/OpenBMC version, and VM configuration.
- [ ] **QSP-T02** Boot from a clean snapshot and capture redacted service-root, UpdateService, TaskService, SoftwareInventory, session/auth, and event capability fixtures.
- [ ] **QSP-T03** Exercise capability discovery and a safe known-image update in isolation; record upload/request bytes, timings, response headers/body classes, returned identities, reboot behavior, and final inventory/health.
- [ ] **QSP-T04** Determine push versus pull delivery. For push, measure S3-to-worker-to-BMC streaming requirements. For pull, prove the BMC supports the URI scheme/TLS path before accepting signed target access.
- [ ] **QSP-T05** Determine whether `Immediate`, `OnReset`, or another mode is supported and identify any additional reset side effect that FleetForge must model and authorize.
- [ ] **QSP-T06** Redact fixtures, classify every field as normative, OpenBMC-specific, or unsupported, and record simulator deviations.
- [ ] **QSP-T07** Rebuild/restore the VM and prove the recovery procedure.
- [ ] **QSP-T08** Shut down resources and record cost.

## Exit criteria

- One update mechanism, apply-time mode, artifact path, image contract, task/inventory evidence hierarchy, and recovery procedure are accepted in ADR-013/ADR-032.
- Phase 04 can build a simulator from observed fixtures without inventing convenient target behavior.
- Phase 09 has a bounded upload/request design and does not assume that a BMC can pull a S3 URL.
- Any missing task/event/idempotency behavior is explicitly unsupported and receives a conservative adapter policy.

## Stop conditions

- Stop before update if image provenance, recovery, credentials, trust, or target isolation is unclear.
- Do not replace the pinned image merely to make an architectural assumption pass. Correct the assumption and record the limitation.
