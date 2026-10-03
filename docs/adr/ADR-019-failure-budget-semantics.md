# ADR-019 — V1 failure-budget and advancement semantics

- **Date:** 2026-10-03
- **Status:** Accepted
- **Owner phase:** 07 / V1-P03

## Decision

V1 stores a non-negative exact rational failure threshold `numerator / denominator`
where the denominator is positive and the numerator does not exceed it. At
canary, wave, and campaign evaluation, terminal failures reach the budget when
`failed * denominator >= numerator * evaluated`; equality is a breach. Zero
failures remain admissible. The existing `policy.EvaluateFailureBudget` is the
single pure evaluator for this rule.

`NEEDS_ATTENTION`, unresolved reservations, nonterminal members, missing,
future-dated, stale, or degraded scheduled health samples block advancement.
Maximum campaign duration pauses rather than skipping unfinished work. V1 does
not implement strict worst-case failure-capacity reservation; requests for that
mode are rejected. Decisions record residual active exposure separately from
the observed terminal threshold.

## Consequences

The same integer rule is used for validation, persistence, evaluation, and
replay. Unknown policy/reducer/algorithm versions fail closed. This does not
claim that an admission already committed before a pause can be cancelled.
