# ADR-018 — V1 deterministic canary selection algorithm

- **Date:** 2026-10-03
- **Status:** Accepted
- **Owner phase:** 07 / V1-P03

## Decision

V1 accepts only `campaign-plan/v1` with selection algorithm `selection/v1`.
Candidate inputs are immutable P02 target IDs, revisions, site/rack labels, and
family. Planning rejects an empty set, more than 10,000 members, duplicate IDs,
or an empty site/rack label.

Canary intent is either an absolute count or integral percentage. Percentages
resolve using ceiling(`candidate_count * percent / 100`). A multi-target plan
must leave at least one later-wave member; a 100-percent request is rejected.
A one-target plan is a one-member canary with a persisted empty later wave.

Rank each target using SHA-256 of length-prefixed algorithm version, seed, and
canonical target ID. First choose the lowest-ranked target from each distinct
`(site,rack)` pair until the resolved canary count is reached, then fill from
the global rank order. Hash collisions use target ID as the stable tie-break.
All remaining ranked targets belong to the single later wave. The persisted
plan records cohort, ordinal, selection reason, effective counts, failure-domain
distribution, algorithm version, seed, and canonical plan hash.

## Consequences

Selection is replayable without inventory I/O and independent of caller order.
The algorithm makes no risk-scoring or external-device claim. Any later
selection behavior requires a new algorithm version and retained replay support.
