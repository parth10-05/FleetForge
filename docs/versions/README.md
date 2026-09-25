# FleetForge Versioned Delivery Roadmap

## Purpose

FleetForge now has a merged Phase 03 persistence foundation. This roadmap packages the remaining work into three demonstrable product versions without weakening the numbered phase contracts in `docs/implementation-plan/`.

A version is a vertical release slice. Completing a slice of a numbered phase does **not** mark the whole numbered phase complete. Each version must preserve the core ordering: persist intent before external work; PostgreSQL remains workflow truth; the queue is only a wakeup; ambiguous firmware submission is reconciled or becomes `NEEDS_ATTENTION`; and a lease expiry never proves a side effect was absent.

## Release map

| Version | Product outcome | Included phase slices | Explicitly deferred |
| --- | --- | --- | --- |
| [v1](v1/README.md) | A safe, local API-driven synthetic-target demonstration of a small firmware campaign. | Static zero-resource IaC, synthetic target contract freeze, Phase 04, and focused slices of Phases 05–10 using provider-neutral local adapters. | Live AWS deployment/provider evidence, browser console, full observability/runbooks, 1,000-target validation, and public portfolio release. |
| [v2](v2/README.md) | An AWS-hosted operator beta with a complete control API and console. | Remaining Phases 05–11; Phase 12 operational baseline; Phase 13 staging/hardening slices. | Release-scale fault/load/restore validation and final public evidence bundle. |
| [v3](v3/README.md) | An evidence-backed portfolio release with measured reliability claims. | Completion of Phases 12–15 and remaining Phase 13 promotion work. | Vendor-wide, real-fleet, physical power-loss, and multi-region claims. |

## Version rules

1. Use the numbered phase documents as the normative contract. This directory only sequences a smaller release scope.
2. Each task has one safety boundary, a concrete output, a test requirement, and a stop condition. Do not bundle several task files into one implementation prompt.
3. A version is not complete because its happy-path demo works. Its listed failure demonstrations and CI gates must pass at the recorded commit.
4. No version may claim support for behavior that its evidence has not measured. In particular, v1 is a small simulator claim, v2 is an operator-beta claim, and v3 is the only version permitted to make measured 1,000-target reliability and efficiency statements.
5. Before moving a task between versions, update the Decision Log, Traceability Matrix, and Risk Register when its owner, invariant, or evidence boundary changes.

## Resume progression

| Version reached | Accurate claim boundary |
| --- | --- |
| v1 | Built a local Go/PostgreSQL firmware-rollout control plane that runs safety-gated campaigns against a deterministic Redfish-shaped synthetic target backend, including persisted work, audit evidence, and ambiguity handling. |
| v2 | Built and deployed an authenticated AWS operator control plane with durable queue-driven orchestration, immutable firmware artifacts, canary/wave controls, and an operator console. |
| v3 | Measured the documented control plane under 1,000 synthetic targets and fault/restore schedules with retained deterministic and independent-journal evidence. |

Replace every descriptive phrase with the exact measured result once evidence exists. Never state `production-ready`, `exactly once`, vendor-wide compatibility, or hardware-flash resilience.
