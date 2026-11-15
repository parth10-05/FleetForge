# ADR-025 — API versioning and compatibility policy

- **Date:** 2026-10-24
- **Status:** Accepted
- **Owner phase:** V1-P06 and Phase 10

## Context

FleetForge is adding its first operator-facing HTTP contract. Clients need a
stable way to distinguish additive evolution from a breaking change, while the
V1 implementation remains a deliberately small local synthetic-target slice.
The OpenAPI document is already the public-contract source of truth under
ADR-008, but it did not yet define the release snapshot or compatibility rule.

## Decision

The public human control API uses a major path prefix: `/v1`.

- A released V1 contract changes only additively. Existing paths, methods,
  operation IDs, required request fields, response field meanings, status-code
  semantics, authentication requirements, pagination/cursor meaning, and
  problem codes are immutable within V1.
- A breaking change requires a new major path and a documented migration
  decision. It must not be hidden behind a permissive decoder or a change to a
  generated transport type.
- Before the first V1 release, `api/openapi/openapi.yaml` may evolve as the
  P06 contract is completed. Closeout snapshots that contract at
  `api/openapi/releases/v1.0.0.yaml`; later V1 changes are compared against
  that snapshot in CI.
- The OpenAPI source, pinned linter, transport-type generator, generated-file
  drift check, and compatibility-diff gate are owned by the control API.
- Generated code is transport-only. Domain, application, persistence, and
  authorization models remain repository-owned explicit types and must never
  be generated from OpenAPI.
- The internal synthetic event-hint route is not part of the human public
  contract. Its HMAC authentication and lifecycle remain separately owned.

## Consequences

Clients can rely on an explicit V1 contract boundary, while implementation
keeps the API adapter thin and prevents generated schemas from defining domain
behavior. The first release gains an explicit baseline for compatibility review.
Maintainers must update the release snapshot only through a reviewed major or
explicitly compatible contract change.

## Failure and rollback implications

If a proposed V1 change breaks the snapshot, CI must fail before publication.
The safe remedy is an additive design, a new major path, or an explicit
superseding ADR; silently weakening validation or rewriting a released snapshot
is not permitted.

## Required evidence

P06 must retain OpenAPI lint, transport-generation clean-tree, operation-ID
uniqueness, internal-route exclusion, and post-release compatibility-diff
results for the exact candidate SHA.
