# ADR-016 — V1 immutable artifact object identity

- **Date:** 2026-09-28
- **Status:** Accepted for V1-P02

## Decision

An artifact binds one immutable object tuple: provider, container, key,
provider-assigned non-empty version ID, declared size, canonical SHA-256, and
verification policy. V1 uses the local versioned-store provider
`local-versioned-v1`; later providers must preserve equivalent exact-version
semantics before they can implement this port.

Digest equality is not object identity. Identical bytes may be retained as
separate artifacts when their version, metadata, or policy tuples differ. Every
HEAD, GET, hold, or deletion request carries the persisted version ID and must
reject a substituted version.

## Consequences

An upload record has no ObjectIdentity until completion observes a version ID.
Verification streams only the bound tuple. New preflight requires a verified,
non-retired, non-expired tuple; existing frozen evidence is immutable.

## Safety and rollback

No age-only lifecycle rule deletes a verified or referenced object. If a store
cannot return and enforce a non-null version ID, artifact ingestion is disabled
rather than weakening the identity contract.
