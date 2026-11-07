# V1-P06 authenticated API and demo evidence package

## Status

`NEEDS_ATTENTION` — this directory defines the retained-evidence contract for
V1-P06. It contains no execution result and must not be treated as a release
claim. V1-P06 can be promoted only after an exact-final-SHA hosted workflow
and its retained artifacts have been inspected.

## Required retained record

Each acceptance run must retain a redacted, SHA-bound index containing only:

- the evidence schema version, candidate SHA, merge-base SHA, workflow URL,
  workflow run identifier, and artifact checksums;
- the deterministic scenario identifier and seed;
- API, schema, simulator-fixture, and IaC version identifiers;
- the executed command names, exit statuses, and explicit skips;
- redacted counts/digests for audit, outbox, due-work, projections, and
  simulator journal checkpoints; and
- the asserted invariant and outcome for healthy, lost-event, post-accept
  ambiguity, canary-stop, and attention-resolution scenarios.

The index must distinguish local working-tree results from hosted exact-SHA
results. A skipped job, missing artifact, non-final SHA, expired artifact, or
unredacted record leaves this package at `NEEDS_ATTENTION`.

## Prohibited fields

No retained record may contain a database URL, credential, bearer token,
private token-file path, signed upload URL, callback payload, target address,
artifact bytes, raw simulator journal record, or raw request/response body.

## Closeout inventory

Before closure, retain and inspect:

1. the exact candidate and resulting-main hosted CI records;
2. one redacted index for every required deterministic acceptance scenario;
3. evidence that duplicate/replayed commands do not produce duplicate durable
   effects;
4. evidence that stale revisions and expired/invalid identities fail safely;
5. a redaction scan and explicit list of unavailable local/hosted gates; and
6. the local environment and resource-use boundary, without a production,
   provider, external-device, browser-console, or scale claim.
