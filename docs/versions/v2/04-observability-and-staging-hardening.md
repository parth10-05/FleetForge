# V2-P04 — Observability Baseline and Staging Hardening

## Goal

Make the operator beta diagnosable and deployable in staging. This phase establishes a measured operational baseline; it does not yet claim final SLO attainment or release-scale proof.

## Tasks

- [ ] **V2-P04-T01** Centralize OpenTelemetry resource fields and propagate safe correlation through HTTP, database, outbox, SQS, worker, Redfish, and SSE boundaries.
- [ ] **V2-P04-T02** Define structured-log allowlists, redaction canaries, bounded metric labels, database/queue/worker/API/web instrumentation, and telemetry self-observation.
- [ ] **V2-P04-T03** Provision initial Grafana Cloud dashboards for fleet, workflow, device, persistence, API/web, and cost/capacity views. Distinguish zero from no data.
- [ ] **V2-P04-T04** Define SLO formulas before measurement, label all unmeasured targets honestly, and implement recording queries for API latency, due-work delay, audit completeness, reconciliation freshness, and outbox age.
- [ ] **V2-P04-T05** Create actionable alerts and initial runbooks for ambiguous update, automatic pause, overdue reconciliation, outbox/queue stall, lease/reservation leak, database saturation, integrity failure, telemetry outage, and cost warning.
- [ ] **V2-P04-T06** Trigger each initial alert in staging with simulator faults, record diagnosis/recovery friction, and revise the dashboard/runbook without hiding false or missing signals.
- [ ] **V2-P04-T07** Complete Terraform/OpenTofu state, module/environment, scan/plan, IAM, network, SQS/S3/Secrets Manager, Neon, Cognito, Grafana, ECR, and environment configuration controls required for staging.
- [ ] **V2-P04-T08** Build once in CI, sign/attest where available, deploy exact digests, run migrations as a dedicated direct-connection job, execute staging smoke/E2E/fault subsets, and retain artifacts.
- [ ] **V2-P04-T09** Rehearse application rollback after an expand migration and a defined health regression. Block unexpected destroy or IAM expansion; never rollback by destructive down-migration.
- [ ] **V2-P04-T10** Verify budget alerts, min/max capacity, queue caps, storage retention, telemetry limits, start/stop controls, and safe disposable teardown/recreate.

## Exit gate

An on-call operator can trace a staging campaign end to end, receive one actionable alert per modeled failure class, follow a tested runbook, and roll back the application while preserving compatible durable state.
