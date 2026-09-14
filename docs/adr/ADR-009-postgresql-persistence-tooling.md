# ADR-009 — Explicit PostgreSQL tooling: pgx, sqlc, and goose

- **Date:** 2026-09-14
- **Status:** Accepted
- **Owner phase:** 03

## Context

Phase 03 needs visible PostgreSQL transaction, SQLSTATE, migration-history, and
query contracts. An ORM would hide the lock and query semantics that the
persistence invariants depend on. Application connections and migration
connections also have different pooling and session requirements.

## Decision

Use `pgx/v5` and `pgxpool` for PostgreSQL access. Store reviewed, explicit SQL
under `db/queries/` and use `sqlc` configured for pgx/v5 to generate types in a
persistence-only package. Use immutable, sequential SQL migrations through the
goose Provider. The migration command connects through a direct PostgreSQL
endpoint and obtains an explicit session advisory lock before applying files;
the application pool never acquires that lock and never auto-migrates.

Exact module and generator versions are pinned in the Go module and generator
configuration. Generated types cannot be imported by `internal/domain`.

## Consequences

- SQL, locking, and index access patterns remain reviewable and testable.
- Migrations have one serialized, observable execution path.
- The project owns mapper validation between generated persistence types and
  Phase 02 domain values.
- CI proves PostgreSQL correctness using an ephemeral service. A Neon
  pooled/direct endpoint smoke test is deferred to the architecture-freeze
  cloud-bootstrap slice and is not claimed by Phase 03.

## Failure and rollback

Application rollback uses the schema compatibility window, not destructive
down-migrations. A failed migration leaves history and diagnostics intact for
operator investigation. A connection or schema-compatibility failure makes
readiness unavailable; it does not cause an application binary to migrate.

## Required evidence

- Empty-schema and supported-snapshot migration tests.
- Concurrent migrator test proving session-lock serialization.
- Reproducible sqlc generation with no generated-file diff.
- Separate pooled application and direct migration configuration tests.
