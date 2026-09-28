# ADR-017 — V1 synthetic artifact verification policy

- **Date:** 2026-09-28
- **Status:** Accepted for V1-P02

## Decision

V1's supported policy is `synthetic-sha256-v1`: a non-empty
`application/octet-stream` object no larger than 16 MiB is streamed through a
bounded SHA-256 verifier and compared to the immutable declared digest. The
verification record retains the policy version, exact object identity, byte
count, safe result code, and timestamp.

Cryptographic signatures and manifests are unsupported in V1. A policy that
requires either fails closed and cannot report `VERIFIED`.

## Consequences

Firmware bytes never traverse the Control API, PostgreSQL, audit, outbox, or
diagnostic payloads. Compatibility remains a deterministic policy over trusted
normalized target facts and verified artifact metadata.

## Safety and rollback

Digest verification is never bypassed for an operator override. If streamed
verification cannot complete within configured byte/time bounds, retain a safe
rejected or unavailable result and do not make a new campaign ready.
