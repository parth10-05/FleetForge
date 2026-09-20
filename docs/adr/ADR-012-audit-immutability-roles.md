# ADR-012 — Database-enforced audit append-only boundary

- **Date:** 2026-09-14
- **Status:** Accepted
- **Owner phase:** 03

## Context

INV-07 requires every lifecycle mutation to retain same-transaction audit
evidence. Repository convention alone cannot prevent an application defect or
future maintenance command from modifying history.

## Decision

Create separate migration, application, and read-only diagnostic database
roles. The application role can insert and select audit rows but has no update
or delete privilege. The diagnostic role is read-only. The migration role owns
schema changes and is used only by the dedicated migration command. Application
code still treats audit records as append-only and validates/redacts payloads
before a transaction begins.

## Consequences

- Privilege-negative integration tests prove the trust boundary.
- Application SQL remains unable to amend historic audit data even if a
  repository method is mistakenly added.
- Operational role provisioning becomes a required migration/deployment step.

## Failure and rollback

Failure to establish the required grants blocks readiness rather than falling
back to a more privileged application role. Restoring a database must restore
audit history and role grants together.

## Required evidence

- Application-role update/delete attempts fail.
- Diagnostic-role mutation attempts fail.
- Aggregate/audit rollback and completeness tests pass against real PostgreSQL.
