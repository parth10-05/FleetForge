# V3-P04 — Portfolio Evidence Release

## Goal

Package FleetForge so a technical reviewer can reproduce the safe simulator demo, inspect the hard correctness work, understand measured limits, and distinguish demonstrated behavior from future work.

## Tasks

- [ ] **V3-P04-T01** Complete repository hygiene: full-history secret scan, dependency/license/provenance review, removal/redaction of private identifiers, pinned toolchains, immutable tag, image digests, SBOM, and evidence retention.
- [ ] **V3-P04-T02** Write outcome-first README, architecture overview, state/ambiguity diagrams, ADR index, security model, limitations, and exact service-choice rationale without claiming unsupported provider behavior.
- [ ] **V3-P04-T03** Publish a 10–15 minute seeded demo guide for healthy rollout, lost event, post-accept ambiguity, and canary stop. Include reset/cleanup, cost, troubleshooting, and simulator-only labeling.
- [ ] **V3-P04-T04** Create a machine-readable evidence manifest mapping each
  correctness, scale, parallelism, security, cost, CI/CD, restore, and
  deterministic-contract claim to exact test/report/commit/environment/date
  evidence.
- [ ] **V3-P04-T05** Publish concise reports for load, parallel efficiency,
  fault, restore, deterministic replay, dashboards/runbooks, and at least one
  real implementation-fault postmortem with original failure, diagnosis, fix,
  and retest.
- [ ] **V3-P04-T06** Verify all documentation links, Mermaid diagrams, commands, screenshots, fixture licenses, and public artifacts on a clean machine/runner.
- [ ] **V3-P04-T07** Ask an external technical reviewer to reproduce the public simulator demo and record friction; fix or document every result without hiding failed runs.
- [ ] **V3-P04-T08** Draft resume bullets and 30-second, 2-minute, and 10-minute explanations only from measured evidence. Include a direct limitations/future-work answer.
- [ ] **V3-P04-T09** Tag the portfolio release only after CI/manual gates pass and the owner accepts the claim/evidence review. Preserve the tag as evidence while later work continues normally.

## Exit gate

FleetForge is reviewable as an honest systems portfolio project: its demo is
reproducible, difficult safety properties are visible, quantitative claims are
traceable, and the synthetic target boundary and untested external-device
behavior are explicit.
