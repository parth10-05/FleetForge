# v2 — AWS Operator Beta

## Outcome

v2 turns the v1 simulator demo into a bounded AWS operator beta. It completes the usable control API and browser console, broadens inventory/artifact/planning policy coverage, hardens staging deployment, and adds an operational baseline without claiming release-scale validation.

## Planned scope

- Complete remaining Phase 05 target registration, selector, discovery, drift, and preflight work.
- Complete remaining Phase 06 signature/manifest, compatibility, retention, and artifact operations.
- Complete Phase 07 policy, wave evaluation, active-exposure, and replay behavior.
- Complete Phase 08 queue classes, capacity/backpressure, repair, dead-letter, and deterministic scheduler requirements.
- Complete Phase 09 adapter/events/reconciliation paths for the single frozen target contract.
- Complete Phase 10 OpenAPI compatibility, Cognito RBAC/recent-auth, pagination, SSE, audit, replay, and rate-limit behavior.
- Complete Phase 11 operator console, accessibility, reconnect/stale-state, attention, and audit UX.
- Implement the Phase 12 telemetry baseline: correlated traces, safe logs, bounded metrics, initial dashboards, alert wiring, and selected runbooks.
- Implement Phase 13 hardening through reproducible staging deployment, OIDC, image provenance, migration compatibility, cost controls, and rollback rehearsal.

## v2 exit evidence

- A Cognito-authenticated operator can complete normal, pause/abort, stale-revision, and attention workflows through the console.
- Staging deploys the same immutable image digest that CI tested; rollback and schema compatibility are demonstrated.
- Queue, database, API, worker, simulator, and browser telemetry can reconstruct a campaign without secret leakage.
- The report labels all throughput/SLO figures as measured only in their exact staging scenario. It does not make a 1,000-target or QEMU-update claim.
