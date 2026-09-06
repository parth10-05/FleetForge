# FleetForge Phase 01 — Detailed Implementation Execution Plan

## Status and purpose

**Status:** In progress. Local source, web, and supply-chain gates are complete;
host-level race and container proof plus hosted CI evidence remain pending.

This document turns the normative Phase 01 checklist into an implementation-ready sequence for the current repository. It does not change the approved architecture. It creates the engineering foundation needed by Phases 02–15 while deliberately excluding campaigns, targets, database schema, queues, cloud deployment, Redfish requests, and authentication.

## Verified starting point

- The repository is an existing Git repository on `main`, with remote `origin` configured and a clean working tree at inspection time.
- Tracked content is the RFC, root documentation map, and implementation-plan package only. No Go module, source packages, Node workspace, CI workflow, or container files exist yet.
- The local machine presently exposes Go `1.27.0`, Node `v22.13.0`, pnpm `11.19.0`, Docker `29.4.3`, and Git `2.49.0`. These are observations, not the CI toolchain lock. Exact supported Go and Node LTS versions must be selected, recorded, and pinned when implementation begins.
- `ADR-007` (Go module/package-boundary enforcement) and `ADR-008` (OpenAPI server/client generation approach) are required Phase 01 decisions and have not yet been accepted in the Decision Log.

## Implementation evidence — 2026-09-06

- ADR-007 and ADR-008 are accepted and recorded under `docs/adr/`.
- `go test ./...` and all three role builds pass locally.
- `pnpm install --frozen-lockfile`, strict web typecheck, ESLint, Vitest, Vite
  production build, and high-severity dependency audit pass locally.
- The local Go race suite cannot start because the host C compiler reports that
  64-bit mode is unavailable. The Linux CI job runs the race suite.
- Docker image execution/non-root/read-only-root proof is explicitly deferred
  by the project owner. Dockerfiles remain available, but no local or CI image
  build is scheduled until that decision is reversed.
- No cloud resource, PostgreSQL instance, queue, Redfish call, campaign, or
  authentication workflow has been introduced.

## Recommended Phase 01 decisions

### ADR-007 — one Go module with compile-time boundaries

Adopt one repository-canonical Go module and the proposed `internal/` package layout. Keep the initial dependency direction explicit and small:

```text
delivery (HTTP/process wiring) -> application (use cases/ports) -> domain (pure rules)
platform, persistence, redfish, and observability implement or support inward-facing ports
cmd/* composes dependencies only
```

`internal/domain` remains pure: it may use the Go standard library only in Phase 01 and must not import HTTP, SQL, AWS, Redfish transport, logging SDK, or OpenTelemetry packages. Enforce this with a repository-owned dependency test or static rule in the aggregate verification target, rather than relying solely on directory convention.

**Trade-off:** one module is intentionally less isolated than several services, but keeps shared domain types, builds, local development, and later contract evolution inexpensive. Independent `cmd/*` binaries and containers preserve runtime isolation.

### ADR-008 — contract-first OpenAPI with generated boundary types only

Create an OpenAPI 3.1 source document now for the Phase 01 public operational endpoints. Use generation only for transport-level request/response interfaces and the TypeScript client in later API/UI phases; keep generated files out of `domain` and `application`. The first implementation pass must select a generator with reproducible pinned installation, committed generation configuration, and CI drift detection. No runtime framework is chosen merely to accommodate a generator.

**Trade-off:** generated transport types provide a single contract and later frontend safety, at the cost of a small reproducibility surface. The explicit generator acceptance test avoids introducing a hidden developer-machine dependency.

## Target repository shape

Phase 01 creates the following owned files and directories. Empty directories require a `.gitkeep` or short package/README document only when Git needs to retain their intended boundary.

```text
cmd/control-api/                  API bootstrap only
cmd/orchestrator-worker/          worker bootstrap only
cmd/redfish-simulator/            persistent simulator bootstrap only
internal/domain/                  pure rules and boundary documentation
internal/application/             role-neutral configuration/use-case ports
internal/persistence/             placeholder interfaces only; no schema
internal/redfish/                 reserved adapter boundary; no protocol calls
internal/delivery/http/           health and version transport only
internal/observability/           log/metrics setup and allowlist
internal/platform/config/         typed env parsing and validation
api/openapi/                      OpenAPI source and generation configuration
api/events/                       reserved versioned-event contract area
db/migrations/, db/queries/       intentionally empty Phase 01 placeholders
web/                              strict React/TypeScript/Vite shell
test/{contract,integration,e2e,fault,load,fixtures}/
deploy/{terraform,cloud-run,compute-engine,dashboards}/
docs/{adr,runbooks,evidence}/
scripts/, .github/workflows/
```

The existing deployment placeholder names are retained for plan continuity, but implementation documentation must state the frozen AWS deployment map. No Terraform provider resources are added in this phase.

## Runtime contracts and control flow

### Startup

Each role follows one bounded, observable bootstrap sequence:

1. Load role-specific environment variables into a typed configuration object.
2. Validate syntax, ranges, cross-field constraints, and unsafe production flags without rendering secret values.
3. Construct structured logging and build metadata resource fields from an allowlist.
4. Construct only Phase 01 local/platform probes; never contact BMCs and never require PostgreSQL.
5. Mark startup success, expose the role’s health/version semantics, and begin the role-specific process loop.
6. On `SIGINT` or `SIGTERM`, change to draining, stop accepting new work, use a configured bounded deadline, emit shutdown telemetry, then exit non-zero only for unsuccessful termination.

The API accepts only health and version routes. The worker accepts no campaign work; its drain implementation proves the worker lifecycle contract without fabricating queue behavior. The simulator owns explicit state and journal paths and refuses retained-test-run startup if either required persisted artifact cannot be opened and recovered.

### Health endpoints

| Endpoint | Meaning | Must not depend on |
|---|---|---|
| `/health/live` | Process is alive and its event loop is responsive. | PostgreSQL, AWS, Redfish, optional telemetry exporter |
| `/health/ready` | Valid configuration and required Phase 01 local/platform prerequisites are available within a short bounded probe. | Remote BMC reachability or campaign state |
| `/health/startup` | Configuration/bootstrap compatibility is known; it remains failing until initialization completes. | Future migrations or cloud resources |
| `/version` | Non-sensitive version, commit SHA, build time, and dirty indicator. | Any dependency |

Responses must be small, stable, cache-safe, and contain safe reason codes rather than raw dependency or configuration errors. HTTP defaults include explicit read, read-header, write, and idle timeouts plus a bounded request body limit.

### Configuration and secrets

Define a common configuration vocabulary with role-specific required fields. Every setting is classified in documentation and tests as one of: required, safe default, secret reference, or environment-specific limit. Phase 01 configuration includes bind address/port, shutdown deadline, environment, log level, build metadata, readiness-probe timeout, and simulator state/journal paths. It may include optional non-secret references for later platform integrations but cannot require live cloud credentials.

`*.env` files, local overrides, credential files, Terraform state, and generated evidence are ignored. `.env.example` contains names and deliberately fake values only. Startup errors refer to environment-variable names and validation categories, never the supplied secret or URL credential component. Log fields use an explicit allowlist; unknown configuration values are not formatted structurally or via `%+v`-style dumping.

## Implementation sequence

| Order | Task IDs | Implementation work | Acceptance evidence |
|---:|---|---|---|
| 1 | P01-T01–T06 | Preserve the current history; add repository metadata, ignore rules, line-ending/generated-file rules, contribution/security policy decisions, and a deferred remote-review-owner note. | Inventory record, clean `git status`, ignored-secret test, documented code-owner deferral. |
| 2 | P01-T07–T12 | Accept ADR-007; create the Go module, three minimal `cmd` entrypoints, package-boundary documentation/enforcement, reusable process lifecycle, and injectable build metadata. | All three binaries build; forbidden-import test; signal/drain tests; `/version` smoke output. |
| 3 | P01-T17–T22 | Implement role-specific typed config parsing, role validators, safe startup diagnostics, `.env.example`, redaction/allowlist policy, and secret-scan guidance. | Positive and negative config unit table tests prove deterministic safe errors. |
| 4 | P01-T23–T27 | Implement API health/version handler and shared readiness/startup lifecycle; add a non-business worker drain loop and simulator state/journal recovery gate. | Liveness/readiness/startup integration tests; bounded shutdown test; retained-simulator recovery test. |
| 5 | P01-T13–T16 | Scaffold the strict pnpm web workspace with feature, API client, design system, routing, and test boundaries. Add runtime public-config injection and a minimal metadata/connectivity shell. | Locked dependency install, strict typecheck, unit test, production build, public-config leakage test. |
| 6 | P01-T28–T31 | Create direct-tool-backed Make targets and Windows-safe script wrappers. Document database-free developer mode and non-secret optional-cloud configuration. | Fresh local clone/repository smoke using paths with spaces; no local database process starts. |
| 7 | P01-T32–T38 | Add path-aware pull-request CI, immutable action pins, dependency caches keyed by lock/module hashes, cancellation, results upload, secret scanning, dependency review, and vulnerability severity policy. | CI workflow syntax/action-pin checks; a deliberately failing test confirms results upload preserves failure. |
| 8 | P01-T39–T42 | Add API, worker, and simulator multi-stage Dockerfiles, non-root runtime user, declared writable temp/state locations, and image inspection scripts. | Container UID, read-only-root-filesystem, health, size, and contents evidence. |
| 9 | all | Produce sanitized evidence, update the Phase 01 checklist, Decision Log, traceability mappings, and known-limitations record. | Reproducible aggregate verification on a fresh hosted runner. |

Tasks are completed in this order even where files are added together. No later task may introduce a campaign, device, schema, queue, cloud, or authentication abstraction simply to make a placeholder look realistic.

## Interfaces and ownership

| Component | Owns | May depend on | Must not depend on |
|---|---|---|---|
| `cmd/*` | dependency wiring, process signal registration | all role-approved implementation packages | business rules or transport details beyond wiring |
| `internal/platform/config` | parsing, normalization, validation, safe field classification | standard library, narrowly scoped parsing helpers | HTTP server, SQL, cloud SDKs, secret values in errors |
| `internal/delivery/http` | request limits/timeouts, health/version responses | application health contracts, safe build metadata | domain transitions, persistence, Redfish |
| `internal/application` | bootstrap lifecycle contracts and future ports | domain, platform abstractions | concrete HTTP/SQL/AWS/Redfish clients |
| `internal/domain` | pure invariants and later state types | standard library only in Phase 01 | delivery, config, persistence, cloud, telemetry |
| `internal/observability` | log field allowlist and bounded process/HTTP telemetry | standard library and selected telemetry facade | secrets, unbounded entity identifiers |
| `web` | display-only metadata and API connectivity state | generated/public API boundary, injected public config | BMC credentials, database state, direct Redfish access |

## Test plan

The implementation pass writes tests alongside each contract, with fault boundary and oracle specified before the implementation that satisfies it.

| Test | Scope | Required oracle |
|---|---|---|
| P01-UT-01 | API, worker, simulator valid configurations | typed config; bootstrap succeeds without PostgreSQL or cloud credentials |
| P01-UT-02 | missing, malformed, contradictory, range-invalid configuration | stable category/variable-name error; supplied secret never appears |
| P01-UT-03 | API and worker signal during idle/drain | new work is rejected/stopped; exit occurs before configured deadline |
| P01-IT-01 | optional probe/exporter unavailable | liveness stays healthy and reports no sensitive diagnostic |
| P01-IT-02 | required Phase 01 readiness probe unavailable | readiness fails promptly with safe reason; liveness remains healthy |
| P01-IT-03 | simulator retained state/journal missing or unrecoverable | startup fails closed; neither artifact is silently recreated |
| P01-CT-01 | each container | effective UID is non-zero and exposed role semantics remain functional |
| P01-CT-02 | each container with read-only root filesystem | only documented temporary/state path is writable; process starts and health behaves correctly |
| P01-WEB-01 | web workspace | strict typecheck, test suite, lint, and production build pass |
| P01-CI-01 | clean hosted runner | aggregate verification passes with only checked-in manifests and CI-provided tools |

Run Go race tests for role lifecycle and shared config code. The Phase 01 suite is intentionally database-free; real PostgreSQL concurrency work begins in Phase 03 and must not be substituted with mocks later.

## Developer and CI command contract

The Makefile exposes explicit, discoverable targets that delegate to visible Go/pnpm/container commands: `format`, `lint`, `test`, `test-race`, `build`, `web-typecheck`, `web-test`, `web-build`, `container-build`, `secret-scan`, and `verify`. CI invokes the same `verify` composition rather than maintaining a divergent hidden command sequence. The final command names and supporting scripts must be tested from a Windows directory containing spaces and a Linux CI runner.

CI runs Go and web paths separately where changes permit, but changes to root manifests, shared scripts, workflows, generated contract configuration, or aggregate build inputs run both. Caches are keyed by `go.sum`, pnpm lockfile, OS, and explicit toolchain inputs. Third-party actions are pinned to immutable commits with an update policy. Test-result upload runs with failure preservation so an upload problem cannot convert a red test suite into a green job.

## Container plan

Each Go role receives a reproducible multi-stage Dockerfile or one parameterized build definition with distinct final targets. Builder stages are allowed toolchains and source; final stages contain only the static/dynamic runtime requirements, non-root executable, CA material where required, and declared writable directories. API/worker remain compatible with read-only root filesystems using a bounded temporary directory. The simulator’s state/journal mount is explicit and durable; it is not conflated with temporary storage.

Image inspection runs in CI and records size, user, filesystem-write behavior, package/source/cache absence, and health smoke results. This phase produces CI artifacts only; it does not push or deploy production images.

## Security, observability, and limitations

- Secrets are blocked by ignore rules, examples, scanning, allowlisted startup diagnostics, and tests that inject canary values.
- All Phase 01 HTTP endpoints are public operational endpoints only. No operator/business route exists yet; Phase 10 owns authentication and authorization.
- Startup/shutdown events, HTTP status/duration/in-flight measurements, process/runtime metrics, configuration-validation failures, and build metadata are emitted with bounded labels. Target, campaign, URL credential, firmware byte, and arbitrary error labels are prohibited.
- An optional telemetry export failure must not block startup or turn liveness unhealthy. Its bounded drop/failure accounting must be observable.
- The plan makes no availability, throughput, cloud-cost, image-size, or deployment claim until measurements exist in evidence artifacts.

## Exit gate, rollback, and stop conditions

Phase 01 is ready to close only when the original exit criteria, all tests above, a fresh-runner `verify`, container evidence, secret scan, health/shutdown smoke output, toolchain/lockfile manifest, and known-limitations record are present and linked to a commit SHA.

Rollback is a single scoped foundation revert after preserving the evidence that exposed the failure. Stop immediately if repository history/user files would be overwritten, a required live cloud credential becomes necessary for the fast suite, a proposed toolchain is unreliable across Windows and Linux CI, ADR-007/ADR-008 cannot be accepted, or the simulator cannot prove retained state/journal recovery. Escalate such a condition through the Decision Log and do not paper over it with in-memory state, broad retries, or test-only bypasses.

## Implementation approval gate

Before code generation begins, the project owner must approve:

1. ADR-007 and ADR-008 recommendations above.
2. The exact Go and Node LTS pins selected at implementation time and the immutable CI action/tool installation pins.
3. The developer-mode contract: no laptop PostgreSQL or cloud credential requirement.
4. The first implementation batch (repository metadata, module scaffolding, typed configuration, health semantics, and tests) before CI/container work.
