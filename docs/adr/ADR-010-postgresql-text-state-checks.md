# ADR-010 — PostgreSQL states use text with named check constraints

- **Date:** 2026-09-14
- **Status:** Accepted
- **Owner phase:** 03

## Context

Phase 02 owns the legal vocabulary and fails closed for unknown values. The
database must reject invalid persisted states while retaining an
expand/migrate/contract path for future state evolution and application
rollback compatibility.

## Decision

Store lifecycle and work states as bounded `text` columns guarded by named
`CHECK` constraints. Every persistence mapper passes loaded values through the
corresponding strict Phase 02 parser or invariant validator. Unknown text,
unknown schema versions, and malformed JSON are integrity failures, never
coerced defaults.

## Consequences

- Constraint names give integration failures and migration reviews stable
  diagnostics.
- Adding a state uses an explicit compatible migration instead of PostgreSQL
  enum mutation semantics.
- Database validation complements rather than duplicates the domain reducer.

## Failure and rollback

An invalid stored value takes the affected aggregate out of service with a
corruption signal. New state values are deployed only after readers accept
them; application rollback occurs before any contract migration that removes
old compatibility.

## Required evidence

- Integration tests reject invalid state text and invalid counters.
- Mapper tests reject unknown states and schema versions.
- Migration inventory records every named state constraint.
