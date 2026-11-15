# V1-P06 API acceptance runner

## Scope

Add a `postgresintegration` acceptance package that composes the real public
control API, local object store, Redfish simulator, and campaign-validation
worker against one isolated PostgreSQL schema per scenario. It must call public
HTTP routes after bootstrap; it must not write campaign, plan, or target-update
state directly.

## Shared fixture

1. Use `test/support/postgres.Harness` and `RunMigrations` for a unique schema.
2. Build and start the repository's local object-store, simulator,
   orchestrator, discovery, wakeup, precheck, execution, observation,
   reconciliation, and control-API binaries with private temporary roots and
   free loopback ports. Give each process a bounded context and always stop it.
3. Mint short-lived local HS256 principals with only the role required for each
   request. Never write token values, DSNs, credentials, target addresses, or
   raw HTTP bodies to logs or retained evidence.
4. Register synthetic targets through `POST /v1/targets`, initiate an artifact
   through `POST /v1/artifacts:initiate`, upload it through the returned local
   capability, and let the real verification worker make it usable.
5. Create and validate campaigns through `/v1`; wait only on bounded
   projections and durable-work invariants until the validation worker reaches
   `READY` or a documented correctable outcome.

## Cases and oracles

| Case | Simulator fixture | Public action | Required invariant |
| --- | --- | --- | --- |
| Healthy | `healthy.json` | create, validate, start | one immutable plan, canary work, audit/outbox consistency |
| Lost event | `lost-event.json` | start canary | polling remains authoritative and journal/evidence remain consistent |
| Post-accept ambiguity | `post-accept-disconnect.json` | start affected update | at most one simulator acceptance; adoption or `NEEDS_ATTENTION`, never blind resend |
| Canary stop | `canary-health-failure.json` | start canary | pause commit precedes every later-wave admission |
| Attention resolution | controlled ambiguous fixture | administrator resolve route | recent admin identity, revision/idempotency fence, immutable audit evidence |

## Retained evidence

Each completed test may emit a report only from `t.Cleanup` when it has not
failed. Reports contain a case ID, deterministic seed hash, candidate/base SHA,
redacted durable counts/digests, asserted invariant, command exit status, and
`PASS`. A package-level index rejects duplicate/missing cases and any field
containing a URL, token, credential, raw journal payload, or request body.

## CI

Run the package under `postgresintegration`, write evidence to a runner-temp
directory, and upload it only when the exact candidate SHA has completed. A
local working-tree run remains explicitly non-release evidence.
