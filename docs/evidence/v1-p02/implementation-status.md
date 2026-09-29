# V1-P02 implementation status

This is a source-and-evidence index for `V1-P02 — Target Inventory,
Preflight, and Artifact Trust`. It records the current local implementation
state on `codex/v1-p02-inventory-artifact-trust`; it is not a phase-closeout
claim and makes no cloud-provider, external-device, or firmware-submission
claim.

| P02 task | Current evidence | Status | Remaining gate |
| --- | --- | --- | --- |
| T01 registration | `internal/application/inventory`, `internal/domain/inventory`, `internal/persistence/postgres/registration.go` | Implemented locally | Execute tagged PostgreSQL registration proof; compose the application writer in a runnable command. |
| T02 endpoint protections | `internal/redfish/address.go`, `client.go`, `address_test.go` | Implemented locally | Final component proof against the P01 runtime and configured CIDR policy. |
| T03 Redfish discovery bounds/errors | `internal/redfish/{client,decode,discovery,errors}.go`, synthetic integration test | Implemented locally | Run final bounded component/fuzz gates and retain results. |
| T04 observations/last good | `discovery_observation.go`, P02 tagged observation proof | Implemented locally | Tagged PostgreSQL execution and normalized capability payload completion. |
| T05 durable discovery | `discovery.go`, Q29, active-work index | Implemented locally | Worker claim/call/result composition and crash/retry proof remain open. |
| T06 bounded selectors | `internal/domain/inventory/selector.go`, Q31, `target_selection.go` | Implemented locally | Execute tagged SQL proof and retain the query-plan/large-inventory evidence. |
| T07 frozen preflight | `internal/domain/preflight`, application evaluator, PostgreSQL candidate writer, Q32 drift report | Implemented locally | Runnable composition and real PostgreSQL execution remain required. |
| T08 readiness recheck | strengthened `AcquireReadinessEnrollment` active/quarantine checks | Partial | P03-owned campaign construction and candidate/artifact/freshness recheck wiring remain open. |
| T09 artifact identity/lifecycle | `internal/domain/artifact`, migrations 00008/00010, immutable guards | Implemented locally | Real PostgreSQL migration/guard execution remains required. |
| T10 direct upload | upload initiation Q34; `localartifactstore` signed, persisted grant receipts; `localobjecthttp`; `cmd/local-object-store` | Implemented locally | The control-plane issuance/binding transaction and a real PostgreSQL-backed end-to-end proof remain open. |
| T11 verification work | bounded verifier, atomic `UPLOADED -> VERIFYING` plus due work, exact tuple fence | Implemented locally | Worker lease/stream/result composition and tagged database execution remain open. |
| T12 stable eligibility | immutable verification result, retirement and preflight-state gates | Partial | Readiness transaction must consume these rechecks in the P03 boundary. |
| T13 compatibility | exact synthetic family policy and frozen per-target decision | Implemented locally | Final component proof against all required target profiles remains open. |
| T14 cleanup/orphans | bounded DB expiration, exact local-version delete | Partial | Workflow-authorized exact physical cleanup and read-only orphan report composition remain open. |

## Executed local evidence

- `go test -count=1 ./...` passed at the current branch head.
- `go vet ./...` passed at the current branch head.
- `git diff --check` passed at the current branch head.
- The tagged PostgreSQL package compiles with
  `go test -tags postgresintegration -run '^$' ./test/integration/postgres`.
- `go test -count=1 ./cmd/local-object-store ./internal/platform/config ./internal/delivery/localobjecthttp ./internal/platform/localartifactstore` passed, including a listener composition test and durable signed-grant receipt recovery after reopening the disk store.

## Unexecuted or deferred gates

- The real tagged PostgreSQL suite requires `FLEETFORGE_TEST_POSTGRES_DSN`.
  A direct local execution was attempted and failed solely because that
  required DSN was absent; this is not passing database evidence.
- The direct object-store listener is locally implemented, but no real
  PostgreSQL-backed control-plane-to-listener flow or P03 operator-facing
  campaign readiness flow has been declared complete.
- No AWS/S3/Secrets Manager/Cognito/Neon behavior, external-device
  interoperability, firmware submission, or production deployment claim is
  supported by this artifact.

P02 may only be closed after all listed remaining gates are evidenced on the
final integration SHA. In particular, P03 owns campaign policy/wave/member
construction and production activation; P02 evidence must not be used to
claim a working firmware campaign on its own.
