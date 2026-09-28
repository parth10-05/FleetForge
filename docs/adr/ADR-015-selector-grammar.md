# ADR-015 — V1 inventory selector grammar

- **Date:** 2026-09-28
- **Status:** Accepted for V1-P02

## Decision

V1 selection has the versioned shape
`{"version":"fleetforge.selector/v1","all":[{"key":"site","value":"iad"}]}`.
Only `site`, `rack`, `group`, and `family` are accepted. Values are
case-sensitive, bounded label values. A selector has one to eight terms and is
canonicalized by lexicographic key order before persistence, hashing, or SQL
compilation.

There are no boolean alternatives, negation, wildcard, JSON-path, regular
expression, arbitrary field, or string-SQL operators in V1. Duplicate keys and
unknown keys fail closed. Empty conjunctions are rejected; a controlled
all-demo fixture must instead pass explicit target IDs and cannot rely on a
dynamic selector.

## Consequences

The persistence adapter compiles terms into parameterized exact-match joins and
orders selected target IDs deterministically. A frozen preflight persists the
canonical selector bytes and compiler version, never a dynamic selector alone.

## Safety and rollback

Do not add an operator by accepting it as a no-op. Extend the grammar only by a
new versioned ADR, parser, compiler, query-plan evidence, and negative tests.
If the compiler cannot preserve bounds or parameterization, disable selection.
