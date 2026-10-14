# ADR-040 — V1 local synthetic OIDC boundary

## Status

Accepted for FleetForge v1.

## Context

The v1 execution demo needs a deterministic, offline identity boundary for
the limited admin-resolution flow. The repository intentionally has no hosted
issuer discovery, Cognito user pool, JWKS endpoint, browser login, or cloud
identity evidence. Calling the local verifier “Cognito-compatible” would imply
interoperability that this code and its evidence cannot establish.

## Decision

V1 accepts only repository-configured local synthetic access tokens signed with
one configured HS256 key. The verifier requires a three-part JWT, `alg=HS256`,
an optional `typ=JWT`, a constant-time valid signature, exact issuer, exact
audience/client ID, `token_use=access`, unexpired `exp`, bounded non-future
`auth_time`, and normalized bounded roles. It returns the explicit issuer class
`local-synthetic-hs256`.

The local verifier is a narrow test adapter behind the application token-verifier
port. It is not an implementation of Cognito, OAuth authorization-code flows,
OpenID Connect discovery, JWKS retrieval/cache/rotation, asymmetric JWT
verification, token revocation, hosted login, or browser/session management.
No FleetForge documentation, API response, audit record, or evidence package
may present the local result as Cognito/JWKS interoperability evidence.

## Required local proof

The verifier tests must reject at least malformed tokens, modified signatures,
unsupported algorithms, wrong issuer/audience, expired tokens, future or stale
authentication times, and malformed role values. Passing tests prove only the
synthetic contract described here.

## Deferred provider proof

A provider-backed identity boundary requires a separate decision and evidence
for issuer discovery, asymmetric key selection by `kid`, JWKS cache/refresh and
key rotation, Cognito claim mapping, authorization-code/browser flows, recent
authentication semantics, revocation/error handling, and redacted hosted
integration tests. That work is deferred to the V2 control-API identity scope;
it cannot be inferred from V1’s HS256 verifier.

## Consequences

V1 admin-resolution tests remain reproducible without external credentials,
while the product documentation makes no provider compatibility promise. Any
future provider adapter must preserve the application-level principal contract
without widening which claims or roles the domain trusts.
