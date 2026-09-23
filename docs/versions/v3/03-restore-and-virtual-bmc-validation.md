# V3-P03 — Restore and Virtual-BMC Validation

## Goal

Prove that durable workflow state is reconstructed conservatively and that
the accepted repository-owned target contract remains deterministic across
restart, container, fault, and restore boundaries.

## Tasks

- [ ] **V3-P03-T01** Create an isolated logical backup/export from a known
  active campaign and restore it to a separate recovery-mode environment with
  submission admission disabled and only isolated virtual-target routes.
- [ ] **V3-P03-T02** Run schema, audit, revision, counter, ownership, due-work,
  outbox, lease, reservation, and decision-replay integrity checks.
- [ ] **V3-P03-T03** Rebuild projections and create fresh delivery generations
  only from durable state. Prove reconstruction cannot reissue unsafe update
  submissions.
- [ ] **V3-P03-T04** Restore a database snapshot older than retained simulator
  state. Require reconciliation, adoption, or quarantine before admission can
  be re-enabled and retain the newer target truth.
- [ ] **V3-P03-T05** Record recovery point/time, manual actions, evidence,
  limitations, and replay comparison. Repair mismatches rather than treating
  recovery as a smoke test.
- [ ] **V3-P03-T06** Re-verify pinned DMTF schema checksums, accepted contract
  ADRs, canonical fixtures, scenario schema, and synthetic extension fields.
- [ ] **V3-P03-T07** Validate target registration, discovery, preflight,
  current-version no-op, Service Root, UpdateService, TaskService,
  FirmwareInventory, artifact streaming, authentication, and event behavior
  against the custom backend.
- [ ] **V3-P03-T08** Execute the accepted synthetic update flow and record task,
  reboot, inventory/version, health, timing, journal, and recovery evidence.
- [ ] **V3-P03-T09** Exercise lost-event fallback, task disappearance, version
  contradiction, health failure, post-accept response loss, and deliberate
  duplicate detection without changing backend behavior to mask failures.
- [ ] **V3-P03-T10** Run identical scenarios locally and in the release
  container, compare normalized evidence, and publish precise wording limited
  to the tested synthetic contract.

## Exit gate

An older database state never overrides newer virtual-target truth, every
accepted operation is represented in the independent journal, deterministic
replay passes across supported runtime boundaries, and the release makes no
external-device or vendor compatibility claim.
