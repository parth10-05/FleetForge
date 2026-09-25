# FleetForge Documentation Map

This file is the stable entry point for FleetForge design and delivery documentation.

## Primary documents

| Document | Purpose | Status |
|---|---|---|
| [System Architecture RFC](FleetForge_System_Architecture_RFC.docx) | Approved HLD/LLD, boundaries, state model, storage, reliability, security, and cloud feasibility | Architecture baseline |
| [Implementation Plan Index](docs/implementation-plan/README.md) | Ordered phase map, dependencies, gates, artifacts, and reading paths | Execution baseline |
| [Core Principles and Delivery Contract](docs/implementation-plan/00-core-principles-and-delivery-contract.md) | Non-negotiable engineering rules applied in every phase | Required before implementation |
| [Architecture Freeze](docs/implementation-plan/architecture-freeze.md) | Frozen managed-service map, corrected correctness contract, proof rules, and phase-order constraints | Normative freeze record |
| [Testing Strategy](docs/implementation-plan/testing-strategy.md) | Test taxonomy, environments, evidence format, and quality gates | Cross-cutting |
| [Traceability Matrix](docs/implementation-plan/traceability-matrix.md) | Maps architecture requirements and invariants to implementation phases and proof | Cross-cutting |
| [Risk Register](docs/implementation-plan/risk-register.md) | Project, safety, technical, cloud-cost, and portfolio risks | Living document |
| [Decision Log](docs/implementation-plan/decision-log.md) | Required ADRs and unresolved implementation decisions | Living document |

## Phase documents

| Order | Phase | Primary outcome |
|---:|---|---|
| 01 | [Repository and Engineering Foundation](docs/implementation-plan/01-foundation-and-repository.md) | Reproducible monorepo, toolchain, CI baseline, configuration, health service |
| 02 | [Domain Model and State Machines](docs/implementation-plan/02-domain-model-and-state-machines.md) | Pure, deterministic campaign and target lifecycle with invariant tests |
| 03 | [Persistence, Migrations, Idempotency, and Outbox](docs/implementation-plan/03-persistence-and-transactional-outbox.md) | Transactional PostgreSQL foundation and replayable intent |
| Contract freeze | [ADR-032](docs/adr/032-supported-update-contract.md), [ADR-038](docs/adr/ADR-038-simulator-isolation.md), [ADR-039](docs/adr/ADR-039-synthetic-redfish-update-contract.md), and [resource matrix](docs/evidence/v1-p01/supported-resource-matrix.md) | Freeze the synthetic update, artifact-transfer, task/inventory, fault, and recovery contract before route implementation |
| 04 | [Deterministic Redfish Simulator](docs/implementation-plan/04-redfish-simulator.md) | Protocol-faithful virtual fleet with seeded fault profiles |
| 05 | [Target Inventory and Preflight](docs/implementation-plan/05-target-inventory-and-preflight.md) | Safe discovery, normalized capabilities, selectors, and frozen target snapshots |
| 06 | [Firmware Artifact Integrity](docs/implementation-plan/06-firmware-artifact-integrity.md) | Immutable object metadata, digest/signature verification, compatibility policy |
| 07 | [Campaign Planning, Canary, and Waves](docs/implementation-plan/07-campaign-planning-and-waves.md) | Deterministic rollout plan and failure-budget evaluation |
| 08 | [Orchestration, Leases, Queues, and Backpressure](docs/implementation-plan/08-orchestration-leases-and-queues.md) | Short durable work units with bounded concurrency and repairable delivery |
| 09 | [Firmware Execution and Ambiguity Reconciliation](docs/implementation-plan/09-firmware-execution-and-reconciliation.md) | Safe Redfish update workflow that never blindly repeats uncertain side effects |
| 10 | [Control API, Identity, RBAC, and Audit](docs/implementation-plan/10-control-api-auth-and-audit.md) | Versioned operator boundary with idempotent commands and append-only evidence |
| 11 | [Operator Console and Live Progress](docs/implementation-plan/11-operator-console.md) | Safety-oriented campaign UI, resumable updates, and accessible audit views |
| 12 | [Observability, SLOs, and Runbooks](docs/implementation-plan/12-observability-slos-and-runbooks.md) | Correlated telemetry, actionable alerts, operational dashboards, and recovery procedures |
| 13 | [Cloud Infrastructure and CI/CD](docs/implementation-plan/13-cloud-infrastructure-and-cicd.md) | Reproducible cloud environments, secure deployment, promotion, and rollback |
| 14 | [Load, Fault, Recovery, and Synthetic Target Validation](docs/implementation-plan/14-load-fault-and-recovery-validation.md) | Measured 1,000-target, bounded-parallelism, fault, restore, and deterministic-replay evidence |
| 15 | [Portfolio Release and Engineering Narrative](docs/implementation-plan/15-portfolio-release-and-evidence.md) | Reproducible public release backed by evidence, limitations, and a technical story |

## Navigation rules

1. Start with the Architecture RFC and Core Principles.
2. Follow the dependency order in the Architecture Freeze. Numbered phase filenames remain stable navigation identifiers and do not override corrected dependencies.
3. Do not begin a phase until its entry criteria are true.
4. Do not mark a phase complete until every required test, evidence artifact, rollback check, and exit criterion passes.
5. A later phase may refine an interface but may not silently weaken an earlier invariant. Record the change in the Decision Log and update the Traceability Matrix.
6. Phase documents are implementation plans, not long-form business-logic code. Small schemas, commands, contract fragments, and Mermaid diagrams are allowed only to remove ambiguity.
