# V3-P03 — Restore and QEMU/OpenBMC Validation

## Goal

Prove that durable workflow state can be reconstructed conservatively and that the exact pinned QEMU/OpenBMC target supports the narrow update/resource path claimed by FleetForge.

## Tasks

- [ ] **V3-P03-T01** Create an isolated logical backup/export from a known active campaign and restore it to a separate recovery-mode environment with submission admission disabled and no real target routes/credentials.
- [ ] **V3-P03-T02** Run schema, audit/revision/counter, active-ownership, due-work/outbox, lease, reservation, and decision-replay integrity checks against the restored state.
- [ ] **V3-P03-T03** Rebuild projections and create fresh delivery generations only from durable state. Prove reconstruction cannot reissue unsafe firmware submissions.
- [ ] **V3-P03-T04** Restore a database snapshot older than persistent simulator state. Require reconciliation/adoption/quarantine before admission can be re-enabled, and retain the newer device truth.
- [ ] **V3-P03-T05** Record recovery point/time, manual actions, evidence, limitations, and a replay comparison. Repair any mismatch rather than treating recovery as a smoke test.
- [ ] **V3-P03-T06** Re-verify the pinned QEMU/OpenBMC image source, checksum, VM configuration, network isolation, credential/trust, and capability fixture from V1-P00.
- [ ] **V3-P03-T07** Validate target registration, discovery, preflight, current-version no-op, service root, UpdateService, TaskService, SoftwareInventory, artifact-transfer path, and event behavior against QEMU; redact and retain actual fixtures.
- [ ] **V3-P03-T08** Execute the one explicit supported update only with the reversible/test image and explicit approval. Record task, reboot, inventory/version, health, timing, recovery, and cost.
- [ ] **V3-P03-T09** Exercise lost-event polling fallback and task-disappearance/vendor-difference reconciliation without corrupting the target. Update adapter fixtures and simulator limitations, never simulator behavior to mask a mismatch.
- [ ] **V3-P03-T10** Shut down QEMU resources and publish precise compatibility wording limited to the pinned image and tested resource/update subset.

## Exit gate

An older database state never overrides newer device/simulator truth, and the portfolio contains direct evidence for one bounded QEMU/OpenBMC update path. Discovery alone does not satisfy this gate.
