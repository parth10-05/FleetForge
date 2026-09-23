# v3 — Validated Portfolio Release

## Outcome

v3 earns FleetForge's strongest portfolio claims through retained evidence:
measured 1,000-target virtual-BMC validation, bounded parallelism, full
fault/recovery schedules, isolated restore, deterministic replay, operational
drills, reproducible release material, and an honest public narrative.

## Planned scope

- Complete Phase 12 dashboards, SLO definitions/measurement, alert tests, runbooks, and drills.
- Complete Phase 13 protected promotion, same-digest deployment, migration/app rollback, IaC security, cost caps, and teardown/recreate evidence.
- Complete Phase 14 L0–L6 methodology as budget permits, including L2–L5
  required scale/fault runs, restore against newer simulator truth, retained
  failing seeds, serial/parallel measurements, and exact synthetic
  update/task/reboot/version evidence.
- Complete Phase 15 README, architecture/case study, demo guide, evidence manifest, license/provenance review, external reproduction feedback, and measured resume/interview narratives.

## Implementation order

| v3 phase | Source-phase mapping | Gate |
| --- | --- | --- |
| [V3-P00](00-release-measurement-contract.md) | Phases 12–14 methodology | Every release claim has a frozen workload, oracle, budget, and evidence format. |
| [V3-P01](01-operational-readiness-and-promotion.md) | Remaining Phase 12 and Phase 13 promotion | Alerts, runbooks, protected promotion, and rollback work in staging/demo conditions. |
| [V3-P02](02-scale-and-fault-validation.md) | Phase 14 L0–L5 | Measured simulator scale and fault evidence supports only its exact claim boundary. |
| [V3-P03](03-restore-and-virtual-bmc-validation.md) | Phase 14 recovery and contract sections | Restore and the accepted virtual-BMC path have direct retained evidence. |
| [V3-P04](04-portfolio-evidence-release.md) | Phase 15 | A reviewer can reproduce, inspect, and critique the released portfolio honestly. |

Complete the phases in order. A failed load, fault, restore, or deterministic
contract run is evidence to retain and repair, never a result to hide or rerun
until it disappears.

## v3 exit evidence

- 1,000-target claims specify exact environment, policy, concurrency, workload, seed, cost, commit, and observed limits.
- The independent simulator acceptance journal reports zero blind duplicate accepted operations across the documented fault matrix.
- Recovery-mode restore disables admission and reconciles newer simulator/device truth before work resumes.
- Device claims are limited to the repository-owned synthetic contract and exact tested update/resource subset.
- Every resume or public claim maps to retained test/report evidence; unsupported claims are removed.
