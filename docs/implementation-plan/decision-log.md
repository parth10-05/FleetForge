# Decision Log and Required ADRs

## Purpose

This log tracks implementation decisions that must remain explicit. Create one ADR per item under `docs/adr/` when the owning phase reaches the decision. Do not bury architectural changes in pull-request discussion.

## ADR status model

- **Proposed:** decision framed; implementation blocked if required before phase.
- **Accepted:** chosen and approved.
- **Superseded:** replaced by a newer ADR, retained for history.
- **Rejected:** considered and intentionally not selected.
- **Experimental:** time-bounded implementation used to collect evidence.

## Locked architecture decisions

| ADR | Decision | Status | Change rule |
|---|---|---|---|
| ADR-001 | Modular monolith with separate API, worker, and simulator runtime roles | Accepted by architecture | Requires architecture review to split services |
| ADR-002 | PostgreSQL is workflow truth; Amazon SQS Standard is delivery transport; first-release projections are database-backed | Accepted | Cannot be weakened by provider convenience |
| ADR-003 | Transactional outbox and durable due work bridge transactions to delivery | Accepted | Alternative must prove loss/duplication safety |
| ADR-004 | Repository-owned deterministic synthetic target backend, locally runnable and optionally container-deployed | Accepted after scope refreeze | Changing the target contract or independent journal changes the evidence claim |
| ADR-005 | Ambiguous side effects reconcile or enter `NEEDS_ATTENTION`; no blind retry | Accepted | Safety invariant; change requires architecture reapproval |
| ADR-006 | AWS-first runtime with Neon PostgreSQL, Amazon SQS Standard, Amazon EventBridge Scheduler repair wakeup, S3, AWS Secrets Manager, Amazon Cognito User Pools, Grafana Cloud, and no event broker in v1 | Accepted after pre-freeze review | Provider changes require an ADR and cannot weaken the correctness contract |
| ADR-034 | Submission pause semantics use persisted epoch-fenced admission; pre-commit admissions remain potentially in flight | Accepted | Do not claim an external network-write fence |
| ADR-035 | Work leases and durable physical-operation reservations have separate lifecycles | Accepted | Lease expiry or attention cannot free physical capacity |
| ADR-036 | Queue envelopes include delivery generation while command identity remains stable | Accepted | Repair must survive Amazon SQS Standard duplicate receipt, visibility expiry, and uncertain publish |
| ADR-037 | Target enrollment claims persist for the parent campaign and unresolved operations retain quarantine | Accepted | Target-update terminality alone cannot release ownership |
| ADR-038 | Synthetic target isolation and single-owner retained-run runtime | Accepted | Any added ingress, route, credential, or concurrent owner requires review |

## Required implementation ADRs

| ADR | Decision needed | Owner phase | Evaluation criteria | Default recommendation |
|---|---|---:|---|---|
| ADR-007 | Go module/package boundary and dependency enforcement | 01 | Simplicity, boundary clarity, build/test speed | One module, internal packages |
| ADR-008 | OpenAPI generation/server/client approach | 01/10 | Contract drift, type safety, domain isolation | Contract-first; generated transport types only |
| ADR-009 | PostgreSQL migration/query tooling | Accepted 2026-09-14 | Explicit SQL, migrations, typing, lock visibility | pgx/v5 + sqlc pgx/v5 + goose Provider with direct-session advisory lock |
| ADR-010 | State storage representation: text checks vs PostgreSQL enums | Accepted 2026-09-14 | Evolution/rollback/validation | Bounded text plus named checks and strict domain parsing |
| ADR-011 | Idempotency in-progress/concurrent caller behavior | Accepted 2026-09-14 | Stable response, bounded waiting, failure recovery | Durable lease-owned claim; bounded in-progress response; completed replay |
| ADR-012 | Audit append-only enforcement boundary | Accepted 2026-09-14 | Tamper resistance vs operational simplicity | Separate migration/application/diagnostic roles with application update/delete denied |
| ADR-013 | Infrastructure state and federated delivery | Accepted for code preparation; administrative execution pending | Remote-state protection, credential-free PR validation, protected OIDC apply/teardown | Terraform modules with provider execution deferred |
| ADR-014 | Simulator scenario schema and controllable clock | Accepted for deterministic preparation | Determinism, scale, fault composability | Seeded declarative scenarios |
| ADR-015 | Selector grammar and query compilation | 05 | Safety, bounded complexity, indexability | Whitelisted expression grammar |
| ADR-016 | Object-storage artifact identity and signed upload | 06 | Generation immutability, digest, cloud support | Bind bucket/key/version ID+SHA-256 |
| ADR-017 | Signature/manifest verification scope | 06 | Honest supported formats, key management | One optional declared format; fail closed when required |
| ADR-018 | Canary selection algorithm/versioning | 07 | Determinism, failure-domain diversity, explanation | Stable hash with rack/site stratification |
| ADR-019 | Failure-budget threshold/rounding semantics | 07 | Boundary clarity/replay | Integer count with explicitly defined inclusive rule |
| ADR-020 | Queue classes, semantic command identity, delivery-generation identity, and retry configuration | 08 | Backpressure, redelivery-safe repair, cost | Few workload classes; DB idempotency authoritative |
| ADR-021 | Work lease, durable operation reservation, enrollment claim, and complete lock acquisition order | Accepted early 2026-09-14 | Deadlock avoidance, stale-worker safety, physical blast radius | Row-backed reservations with documented deterministic writer order |
| ADR-022 | Ambiguity evidence hierarchy/proof-of-absence policy | 09 | Safety, synthetic-target observability, operator burden | Conservative hierarchy; inconclusive → attention |
| ADR-023 | Redfish event authentication/correlation | 09 | Supported target behavior, spoofing/replay | Signed secret/network isolation where available; polling remains correctness path |
| ADR-024 | Amazon Cognito User Pools, Google sign-in, application RBAC, and browser session storage | 10/11 | Secure session/token storage, CSRF, revocation, `auth_time` recent auth | Cognito OIDC adapter; server-managed secure cookie or in-memory token flow; no localStorage by default |
| ADR-025 | API versioning and breaking-change policy | Accepted 2026-10-24 | Client compatibility, contract governance | `/v1` plus schema diff; additive within version |
| ADR-026 | SSE projection/resync design | 10/11 | Bounded buffers, gaps, auth | Events as hints; gap triggers authoritative refetch |
| ADR-027 | Metrics/logs/traces backend and cardinality budget | 12 | Free limits, OTel, retention, alerts | Grafana Cloud Free via OTel with bounded labels |
| ADR-028 | IaC state and environment topology | 13 | Security, reproducibility, cost | Remote protected state; dev/staging/demo separation |
| ADR-029 | GitHub OIDC claim/permission model | 13 | No long-lived keys, least privilege | Repo/ref/environment restricted federation |
| ADR-030 | Deployment/migration promotion and rollback | 13 | Same digest, compatibility, safe rollback | Expand/migrate/contract + ECS Fargate revisions |
| ADR-031 | Load tool/scenario/report format | 14 | Reproducibility, distributed load need, cost | Small Go/k6 harness selected from requirements |
| ADR-032 | Supported synthetic target update contract and evidence boundary | Accepted for Contract freeze/14 | Schema provenance, recovery, data path, limits, claim accuracy | One synthetic contract with pinned schemas and independent journal |
| ADR-033 | Public demo/repository/evidence retention | 15 | Security, cost, reviewer reproducibility | Locally reproducible synthetic target demo with retained measured evidence |
| ADR-039 | Synthetic Redfish-shaped firmware update mechanism/version | Accepted for Contract freeze/09 | Selected fields, extension clarity, upload path, task/evidence behavior | One explicit synthetic path before route implementation |

## ADR template

Each ADR contains:

1. Title, date, owner, status.
2. Context and specific decision pressure.
3. Requirements and constraints.
4. Options genuinely considered.
5. Decision and exact scope.
6. Consequences: benefits, costs, operational burden, security, portability.
7. Failure/rollback implications.
8. Evidence or experiment required.
9. Supersedes/superseded-by links.

Avoid long implementation code. Small diagrams, contract fragments, or benchmark tables are permitted.

## Phase 01 accepted ADR records

- [ADR-007 — One Go module with enforced inward package boundaries](../adr/ADR-007-go-module-boundaries.md)
- [ADR-008 — Contract-first OpenAPI with generated transport types only](../adr/ADR-008-openapi-boundary.md)
- [ADR-009 — Explicit PostgreSQL tooling: pgx, sqlc, and goose](../adr/ADR-009-postgresql-persistence-tooling.md)
- [ADR-010 — PostgreSQL states use text with named checks](../adr/ADR-010-postgresql-text-state-checks.md)
- [ADR-011 — Durable, lease-owned idempotency claims](../adr/ADR-011-durable-idempotency-claims.md)
- [ADR-012 — Database-enforced audit append-only boundary](../adr/ADR-012-audit-immutability-roles.md)
- [ADR-014 — Simulator scenarios and logical time](../adr/ADR-014-simulator-scenarios-and-logical-time.md)
- [ADR-021 — Deterministic lock order for persistence writers](../adr/ADR-021-persistence-lock-order.md)
- [ADR-025 — API versioning and compatibility policy](../adr/ADR-025-api-versioning-and-compatibility.md)
- [ADR-038 — Synthetic target isolation and single-owner runtime](../adr/ADR-038-simulator-isolation.md)
- [ADR-039 — Synthetic Redfish-shaped update mechanism](../adr/ADR-039-synthetic-redfish-update-contract.md)

## Decision gates

- Required ADR for a phase must be accepted before code depends on it.
- Experimental ADR has an expiry and success/failure measurement.
- Provider/tool swaps cannot change domain contracts silently.
- Any change to INV-01–INV-12 requires updating the Architecture RFC, Traceability Matrix, Risk Register, and obtaining project-owner approval.
