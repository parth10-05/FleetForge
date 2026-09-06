# Phase 01 — Repository and Engineering Foundation

## Outcome

Create a reproducible monorepo that builds the ECS Fargate API/worker roles, the EC2-hosted simulator role, and one TypeScript web application; has a minimal automated quality gate; validates configuration at startup; exposes correct health semantics; and can be used without running PostgreSQL on the laptop.

This phase proves engineering repeatability. It does not implement campaigns, devices, firmware behavior, queues, or business workflows.

## Entry criteria

- Architecture RFC and Core Principles accepted.
- Architecture Freeze accepted; event broker is excluded from the first-release dependency graph.
- Project directory is available and existing artifacts are preserved.
- Git hosting destination is chosen or explicitly deferred.
- Current stable Go and Node LTS versions are selected and recorded.
- CI runner constraints are known.

## Non-goals

- No campaign or target CRUD.
- No PostgreSQL schema beyond a connectivity placeholder.
- No real cloud deployment.
- No Redfish calls.
- No custom authentication logic.
- No large framework abstraction before a second concrete use exists.

## Proposed repository layout

```text
FleetForge/
├── cmd/
│   ├── control-api/
│   ├── orchestrator-worker/
│   └── redfish-simulator/
├── internal/
│   ├── domain/
│   ├── application/
│   ├── persistence/
│   ├── redfish/
│   ├── delivery/
│   ├── observability/
│   └── platform/
├── api/
│   ├── openapi/
│   └── events/
├── db/
│   ├── migrations/
│   └── queries/
├── web/
├── test/
│   ├── contract/
│   ├── integration/
│   ├── e2e/
│   ├── fault/
│   ├── load/
│   └── fixtures/
├── deploy/
│   ├── terraform/
│   ├── cloud-run/
│   ├── compute-engine/
│   └── dashboards/
├── docs/
│   ├── adr/
│   ├── runbooks/
│   ├── evidence/
│   └── implementation-plan/
├── scripts/
├── .github/workflows/
├── go.mod
├── Makefile
└── README.md
```

Use one Go module initially. Packages enforce boundaries; separate modules would add dependency/version overhead without increasing isolation. Runtime roles share domain packages but have independent `main` packages, containers, configuration, and deployment manifests.

## Technology decisions

| Area | Decision | Reason |
|---|---|---|
| Backend/runtime | Go | Bounded concurrent I/O, worker control, static binaries, race tooling, low runtime footprint |
| Web | React + TypeScript + Vite | Client-side operations console; no second server framework needed |
| Package manager | pnpm with lockfile | Reproducible frontend install and efficient cache |
| API contract | OpenAPI 3.1, generated client allowed | One source for handlers, client types, and contract tests |
| Logging | Go structured logging abstraction compatible with OpenTelemetry | Safe JSON logs and correlation fields from the start |
| Build entry point | Make targets wrapping direct tools | Discoverable commands without hiding underlying invocations |
| Containers | Multi-stage, non-root, minimal runtime image | Reproducible deployables and smaller attack surface |
| Configuration | Environment variables parsed into typed config | Twelve-factor cloud deployment; startup validation |

Pin exact toolchain and action versions during implementation. Do not hardcode a version in this planning document that may be stale when the phase begins.

## Micro-task checklist

### Repository safety and metadata

- [ ] **P01-T01** Inventory all existing FleetForge files and record which are user-authored, generated, or planning artifacts.
- [ ] **P01-T02** Initialize Git only if the project is not already a repository; never overwrite an existing history.
- [ ] **P01-T03** Add `.gitignore` for Go, Node, IDE, coverage, generated evidence, credentials, Terraform state, and local environment overrides.
- [ ] **P01-T04** Add `.gitattributes` for line endings and generated-file classification.
- [ ] **P01-T05** Add license decision placeholder, contribution guide, code-of-conduct decision, and security-reporting policy.
- [ ] **P01-T06** Add CODEOWNERS or an equivalent review-owner file once a Git remote exists.

### Go workspace

- [ ] **P01-T07** Create one Go module with a repository-canonical module path.
- [ ] **P01-T08** Create the three runtime entrypoints with only bootstrap/configuration wiring.
- [ ] **P01-T08A** Ensure the simulator entrypoint supports explicit persistent state/journal paths and fails startup if a retained test run cannot recover them.
- [ ] **P01-T09** Create package-boundary documentation defining allowed dependencies: transport → application → domain; adapters implement ports inward.
- [ ] **P01-T10** Prevent `internal/domain` from importing HTTP, SQL, cloud, Redfish transport, or telemetry SDK packages.
- [ ] **P01-T11** Add graceful shutdown handling with a bounded drain deadline for API and worker roles.
- [ ] **P01-T12** Add build metadata fields for version, commit SHA, build time, and dirty-state indicator.

### Web workspace

- [ ] **P01-T13** Scaffold React/TypeScript/Vite under `web/` with strict TypeScript settings.
- [ ] **P01-T14** Establish feature, API-client, design-system, routing, and test directories without implementing campaign screens.
- [ ] **P01-T15** Define browser configuration injection so environment-specific API/auth values are not compiled from developer secrets.
- [ ] **P01-T16** Add a minimal shell showing build metadata and API connectivity state.

### Configuration and secrets

- [ ] **P01-T17** Define typed configuration structures separately for API, worker, and simulator.
- [ ] **P01-T18** Classify every setting as required, optional-with-safe-default, secret reference, or environment-specific limit.
- [ ] **P01-T19** Fail startup on missing required configuration, invalid duration/URL/range, contradictory settings, or unknown production-dangerous flags.
- [ ] **P01-T20** Provide `.env.example` containing names and safe dummy values only.
- [ ] **P01-T21** Add secret scanning to pre-commit guidance and CI.
- [ ] **P01-T22** Establish log-field allowlisting; config values marked secret must never be formatted into startup errors.

### Health and process semantics

- [ ] **P01-T23** Implement `/health/live` as a process-only check.
- [ ] **P01-T24** Implement `/health/ready` with bounded configuration/platform checks; do not query remote BMCs.
- [ ] **P01-T25** Implement `/health/startup` or equivalent startup status exposing schema/config compatibility once those exist.
- [ ] **P01-T26** Add `/version` with non-sensitive build metadata.
- [ ] **P01-T27** Define worker shutdown behavior: stop accepting new deliveries, finish or abandon within deadline, leave durable work recoverable, and never interpret shutdown or lease expiry as proof that a firmware call did not occur.

### Developer commands

- [ ] **P01-T28** Provide explicit commands for format, lint, unit test, race test, build, web typecheck, web test, container build, and aggregate verify.
- [ ] **P01-T29** Ensure commands work from paths containing spaces on Windows and from Linux CI.
- [ ] **P01-T30** Add a lightweight developer mode that does not require a local database.
- [ ] **P01-T31** Document optional cloud development variables without placing live credentials in shell history examples.

### CI baseline

- [ ] **P01-T32** Add pull-request workflow with path-aware Go and web jobs.
- [ ] **P01-T33** Cache dependencies using lock/module hashes, not mutable branch names.
- [ ] **P01-T34** Run formatting checks, static analysis, unit tests, frontend typecheck/tests, build, and secret scan.
- [ ] **P01-T35** Upload test results even on failure while ensuring an upload failure does not hide the original test status.
- [ ] **P01-T36** Pin third-party actions by immutable commit and record update policy.
- [ ] **P01-T37** Add cancellation for superseded branch runs.
- [ ] **P01-T38** Add baseline supply-chain checks: dependency review where available and vulnerability scan with an explicit severity policy.

### Container baseline

- [ ] **P01-T39** Add separate multi-stage Dockerfiles or parameterized targets for API, worker, and simulator.
- [ ] **P01-T40** Run as non-root with read-only root filesystem compatibility and a documented writable temporary directory.
- [ ] **P01-T41** Include health metadata only where it matches the deployment platform semantics.
- [ ] **P01-T42** Prove images contain no source tree, package-manager cache, cloud credentials, or development tools.

## Required tests

| Test ID | Scenario | Oracle |
|---|---|---|
| P01-UT-01 | Valid config for each role | Parsed typed config and successful bootstrap |
| P01-UT-02 | Missing/invalid required config | Deterministic safe error; no secret value printed |
| P01-UT-03 | Shutdown signal | New work stops and process exits within configured deadline |
| P01-IT-01 | Liveness while optional dependency is unavailable | Liveness remains healthy |
| P01-IT-02 | Readiness with required platform dependency unavailable | Readiness fails quickly with safe reason |
| P01-CT-01 | Container runs as non-root | Runtime UID is non-zero; service remains functional |
| P01-CT-02 | Container filesystem read-only | Service starts with only declared writable temporary path |
| P01-WEB-01 | Frontend strict build/typecheck | No type or lint errors |
| P01-CI-01 | Fresh hosted runner | Full verify succeeds without undeclared machine state |

## Observability introduced

- Startup and shutdown structured events.
- Build/version resource attributes.
- HTTP request count, duration, status class, and in-flight gauge without unbounded labels.
- Process/runtime metrics.
- Configuration validation failure count without configuration values.

## Security checks

- Secret scanner passes on repository history introduced in this phase.
- Dependency vulnerabilities follow documented fail/warn thresholds.
- HTTP server has header/body/read/write/idle timeouts and request-size defaults.
- Debug endpoints are disabled or separately authorized outside development.
- Browser configuration contains only public values.

## CI/CD increment

This phase creates CI, not production deployment. A green commit produces versioned API/worker/simulator binaries, web build output, and container images as ephemeral CI artifacts. No cloud promotion occurs.

## Evidence artifacts

- Repository tree snapshot.
- Toolchain/lockfile manifest.
- First green CI link and commit SHA.
- Container size, UID, and contents inspection.
- Health/shutdown smoke-test output.
- Known limitations record.

## Exit criteria

- Fresh CI runner completes aggregate verification.
- Three Go roles and the web shell build reproducibly; API/worker containers and the persistent simulator VM process have distinct deployment assumptions.
- Health, shutdown, config-negative, non-root, and secret-scan tests pass.
- No laptop database is required.
- Package-boundary rules are documented and accepted.
- No campaign/device behavior has leaked into bootstrap packages.

## Rollback and stop conditions

- Revert the phase as one foundation change if toolchain selection makes Windows development or Linux CI unreliable.
- Stop if repository initialization would overwrite existing Git history or user files.
- Stop if required cloud credentials become necessary merely to run unit verification; keep cloud integration behind later tagged tests.
