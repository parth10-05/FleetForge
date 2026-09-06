# ADR-008 — Contract-first OpenAPI with generated transport types only

- **Date:** 2026-09-06
- **Status:** Accepted
- **Owner phase:** 01 and 10

## Decision

`api/openapi/openapi.yaml` is the source of truth for public HTTP contracts.
Generation is permitted only for transport request/response types and later web
client types. Generated artifacts cannot become domain or application models.
The generator selection is deferred until a reproducible pinned tool,
configuration, committed output policy, and CI drift check are ready.

## Consequences

The foundation receives a stable contract without selecting a server framework
for generator convenience. Phase 10 must add generation and contract-drift CI
before exposing business operations.

## Rollback

Any change of source contract or generator requires an ADR update and an
explicit compatibility note.
