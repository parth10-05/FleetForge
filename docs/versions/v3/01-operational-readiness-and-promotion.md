# V3-P01 — Operational Readiness and Protected Promotion

## Goal

Complete the operational evidence and protected promotion path required to run v3 validation safely and recover from known deployment or telemetry failures.

## Tasks

- [ ] **V3-P01-T01** Complete trace sampling/linking, safe log retention, metric cardinality enforcement, telemetry outage behavior, and critical trace retention for rare safety outcomes.
- [ ] **V3-P01-T02** Complete all seven dashboard groups, deployment/campaign/fault annotations, no-data states, safe trace/log links from the console, and evidence export retention.
- [ ] **V3-P01-T03** Finalize SLO numerator/denominator/window definitions and trigger every alert/recovery condition using controlled staging data, including never-started eligible due work.
- [ ] **V3-P01-T04** Execute and revise all runbooks: ambiguity, budget pause, queue/outbox stall, database saturation, lease/reservation repair, lost event, rollback/schema, credential rotation, cost teardown, and isolated restore.
- [ ] **V3-P01-T05** Configure protected demo-production environment approvals, same-digest promotion, synthetic read/safe-write checks, and an explicit separation between app deployment and campaign-start credentials.
- [ ] **V3-P01-T06** Prove GitHub OIDC/IAM restrictions, ECR image immutability, migration lock, expand-migrate-contract compatibility, previous-app compatibility, and traffic rollback under defined regression.
- [ ] **V3-P01-T07** Run disposable-environment teardown/recreate and retained-environment backup/export preparation while preserving audit/replay evidence and secret boundaries.
- [ ] **V3-P01-T08** Capture an operational drill report for each page alert, including detection time, diagnosis, action, recovery, false/missing signals, and resulting improvement.

## Exit gate

v3 validation runs can be stopped, observed, diagnosed, rolled back, and recovered through tested runbooks and protected infrastructure controls rather than personal knowledge.
