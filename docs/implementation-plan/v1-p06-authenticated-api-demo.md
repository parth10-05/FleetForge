# V1-P06 implementation map — authenticated API and reproducible demo

## Status and base

This plan is executable only from integrated main commit
`46b5482cbcd6f6573d095133e419d5268217dd9f`. Its P05 parent is verified by
[Foundation verification #157](https://github.com/parth10-05/FleetForge/actions/runs/40072571287).
The implementation branch is `codex/v1-p06-authenticated-api-demo`.

V1 remains a local, repository-owned, synthetic-target-backed demonstration.
PostgreSQL remains workflow truth. No HTTP handler may contact a target, read
or write artifact bytes, sleep, poll, execute campaign planning, or retain a
database transaction over any external operation.

## Binding decisions

- ADR-040 controls identity: configured local HS256 bearer tokens only. Do not
  claim Cognito, OIDC discovery, JWKS, asymmetric JWT, browser, session, or
  federation compatibility.
- ADR-025 controls public compatibility: `/v1`, additive-only evolution after
  the v1 snapshot, OpenAPI as the contract source, and generated
  transport-only types.
- `POST /v1/events/synthetic` remains a separate service-HMAC route. It is
  deliberately excluded from the human OpenAPI contract and cannot accept a
  bearer token; human routes cannot accept its HMAC.

## Delivery order

### Run 1 — contract and tooling

**Files:** `api/openapi/openapi.yaml`, `api/openapi/.spectral.yaml`,
`api/openapi/oapi-codegen.yaml`, `internal/delivery/controlhttp/openapi_types.gen.go`,
`Makefile`, `.github/workflows/foundation.yml`, and pinned web tooling.

Publish health/version plus the bounded `/v1` target, artifact, campaign, and
attention operations. Every human operation declares bearer security, an
operation ID, bounded success/error responses, and RFC 9457 problems. Add a
pinned Spectral check, a pinned `oapi-codegen` transport generation command,
and clean-tree drift checks. Generated types must not define application,
domain, persistence, authorization, or server behavior.

**Exit tests:** OpenAPI parses and lints; operation IDs are unique; every human
route declares security and a problem response; the internal event route is
absent; regeneration does not change tracked output.

### Run 2 — shared access boundary

**Files:** `internal/application/access/{principal,permissions,authorizer}.go`,
tests in that package, `internal/platform/localoidc/*`, and the P05 execution
attention-resolution adapter.

Introduce a closed `Principal` containing subject, normalized roles,
authentication time, and issuer class. Define a closed role/permission lattice:
viewer reads; operator inherits viewer and controls campaign draft/validate/
start/pause; admin inherits both and controls target registration, artifacts,
and attention resolution. The verifier must build this principal only after a
constant-time HS256 signature validation and bounded issuer, audience, subject,
group, expiry, issued-at/not-before, and auth-time validation.

Migrate `execution.VerifiedPrincipal` through a compatibility adapter or shared
type without weakening P05 recent-auth semantics. Tokens, raw claims, and
verification causes never escape the access boundary.

**Exit tests:** role inheritance and deny-by-default matrices; invalid algorithm,
issuer, audience, key, time and claim cases; exact recent-auth boundary; fuzz
redaction checks; existing P05 resolution tests retain their behavior.

### Run 3 — HTTP kernel

**Files:** `internal/delivery/controlhttp/{router,middleware,decode,problem}.go`,
package tests, and `cmd/control-api/main.go`.

Compose public health/version, isolated synthetic-HMAC, and bearer-authenticated
human chains separately. Middleware order is recovery, correlation parsing,
request bounds, content type and strict decoder, authentication, rate limit,
authorization, handler, and safe response logging. Correlation IDs are one UUID
or server-generated. Header/body/query/page/deadline limits are enforced before
application work. Problems are generic at authentication/authorization and
never reveal existence or secret-bearing values.

Cursor signing belongs behind a provider-neutral port and binds version, query
shape, tuple, issue time, and expiry. Per-process limiting keys unauthenticated
traffic from `RemoteAddr`, never forwarded headers.

**Exit tests:** route/method and credential-crossing matrices; strict JSON and
limit failures; panic/redaction; correlation/ETag/idempotency parsing;
cursor tamper/reuse/expiry; limiter/deadline boundaries.

### Run 4 — command consistency substrate

**Files:** migration `00027_*`, `db/queries/*`,
`internal/persistence/postgres/*`, and PostgreSQL integration tests.

Complete command response references and append-only high-risk denial audit.
Idempotency keeps two transactions: a short claim, then a mutation transaction
that atomically writes aggregate mutation, lifecycle audit, outbox/due work,
and completed idempotency response. The completed reference is bounded and
reconstructs an exact safe response; it never stores a token, URL signature,
raw idempotency key, or request body.

**Exit tests:** duplicate replay, mismatch conflict, abandoned-claim repair,
revision conflict, serialization retry, and failure injection before mutation,
before completion, and before commit; no partial persistent effects.

### Runs 5–7 — thin feature adapters

Add target register/discovery and projections; artifact initiation/status; then
campaign draft/validation/projections/start/pause and attention resolution.
Each handler maps exactly one DTO to one application command or query. Existing
state machines, fencing, capacity, retry, audit, and reconciliation logic remain
authoritative. Discovery and validation enqueue durable work and return `202`;
they do not synchronously reach the simulator.

All mutations require `Idempotency-Key`. Existing aggregate mutations also
require `If-Match: "rev-N"`; missing is `428`, stale is `412`. List endpoints
use bounded keyset queries, signed query-bound cursors, and stable tie breakers.
Artifact initiation derives a response-only local capability from immutable
grant metadata; an exact replay reconstructs the original response and does
not persist its signed URL.

**Exit tests:** full endpoint role matrix; existence resistance; duplicate and
stale mutation behavior; ETags; pagination no gaps/duplicates; artifact and
campaign state guards; pause/attention P05 fencing preservation; transactional
audit and safe denial history.

### Run 8 — deterministic CLI demo

**Files:** `cmd/fleetforge-demo/main.go`, `scripts/demo/run-v1.ps1`,
`scripts/demo/stop-v1.ps1`, approved scenario manifests, README documentation.

The demo owns one validated private run directory and only its child processes.
It starts supported local binaries, applies migrations, seeds synthetic targets
and faults, mints short-lived local tokens without printing them, and calls the
HTTP API for all phase operations after bootstrap. It exports only redacted
audit/journal summaries. Stop validates ownership and resolved paths before
removal; it never accepts an API value as a shell command.

**Exit tests:** deterministic seed replay, noninteractive invocation, expired
token, process/port failure cleanup, interruption, and secret-free stdout,
stderr, and evidence.

### Runs 9–11 — acceptance, hardening, closeout

From clean unique seeds, retain healthy, lost-event, post-accept ambiguity,
canary-stop, and attention-resolution evidence. Assert database invariants,
projections, audit/outbox/journal consistency, idempotent replay, and redaction
for every scenario. Run race, fuzz/property, fault, cursor, request-limit, and
bounded-query-plan suites. Document actual environment and limits rather than
making scale, provider, production, or external-device claims.

Closeout adds `docs/evidence/v1-p06/` with exact commit/base SHA, commands,
tool versions, scenario seed, test/CI results, artifact checksums, zero-cloud
attestation, redaction scan, and explicit skips. Hosted CI must be green and
inspected on the exact final candidate and, after authorized integration, the
exact resulting main SHA.

## Stop conditions

Stop and update this plan before implementation if an endpoint requires a new
domain transition, a provider identity feature, a transaction spanning external
I/O, non-atomic mutation/audit/outbox/idempotency completion, a cursor without
query binding, a behavior that weakens P05 fencing, or any unredactable
secret-bearing replay field.
