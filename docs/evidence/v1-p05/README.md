# V1-P05 execution evidence package

## Status

`VERIFIED` — V1-P05 closes only when an exact-final-SHA hosted workflow has
passed and its non-expired `fault-harness-evidence` artifact contains both the
composed and durable PostgreSQL indexes. This directory must not be used as a
substitute for that retained execution record.

## Generated evidence

The `foundation.yml` job retains two disjoint redacted report inventories:
`index.json` for composed simulator cases `P05-F01` through `P05-F16`, and
`postgres-index.json` for durable PostgreSQL cases `P05-F17` through
`P05-F21`. Each report records only:

- schema version and stable case identifier;
- scenario SHA-256 and deterministic seed;
- simulator run identifier and journal checkpoint/count;
- a redacted database-summary digest;
- the asserted invariant and outcome; and
- negative-control and restore outcomes where applicable.

Reports must never contain a DSN, credential, bearer token, callback body,
target address, artifact bytes, or raw journal records. The hosted artifact is
the execution record; committed files document its schema and retention rules.

`fault-matrix.json` is the authoritative catalog of the 21 required case IDs.
Composed and PostgreSQL-only cases retain separate indexes so neither boundary
is falsely represented as the other; both are required toward closeout.

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
