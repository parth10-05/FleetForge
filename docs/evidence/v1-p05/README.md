# V1-P05 execution evidence package

## Status

`NEEDS_ATTENTION` — this directory defines the retained evidence contract. It
does not close V1-P05 and it must not be used as a substitute for a completed
deterministic fault matrix, a restore transcript, or exact-final-SHA CI.

## Generated evidence

The `foundation.yml` fault-harness job must retain a redacted JSON report for
every executed case. Each report records only:

- schema version and stable case identifier;
- scenario SHA-256 and deterministic seed;
- simulator run identifier and journal checkpoint/count;
- a redacted database-summary digest;
- the asserted invariant and outcome; and
- negative-control and restore outcomes where applicable.

Reports must never contain a DSN, credential, bearer token, callback body,
target address, artifact bytes, or raw journal records. The hosted artifact is
the execution record; committed files document its schema and retention rules.

## Required closeout inventory

Before closeout, the package must identify the exact final commit SHA and
workflow run, with artifact names and digests, and provide an index covering:

1. every supported deterministic fault/acceptance case and its seed;
2. both intentional negative controls, each observed to fail its oracle;
3. the retained simulator restart/restore case; and
4. any deferred behavior, stated as deferred rather than passed.

The corresponding hosted artifacts must be inspected after the final push.
An absent report, incomplete matrix, expired artifact, or non-final workflow
run leaves this package at `NEEDS_ATTENTION`.
