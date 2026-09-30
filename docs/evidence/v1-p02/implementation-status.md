# V1-P02 implementation status

This is a source-and-evidence index for `V1-P02 — Target Inventory,
Preflight, and Artifact Trust`. It records the current local implementation
state on `codex/v1-p02-inventory-artifact-trust`; it is not a phase-closeout
claim and makes no cloud-provider, external-device, or firmware-submission
claim.

| P02 task | Current evidence | Status | Remaining gate |
| --- | --- | --- | --- |
| T01 registration | `internal/application/inventory`, `internal/domain/inventory`, `internal/persistence/postgres/registration.go` | Implemented locally | Execute tagged PostgreSQL registration proof; compose the application writer in a runnable command. |
| T02 endpoint protections | `internal/redfish/address.go`, `client.go`, address tests and `FuzzTargetAddressKeepsAcceptedResourcesUnderServiceRoot` | Implemented locally | The 10-second local address fuzz gate retained no service-root escape. Final component proof against the P01 runtime and configured CIDR policy remains open. |
| T03 Redfish discovery bounds/errors | `internal/redfish/{client,decode,discovery,errors}.go`, synthetic integration test, decoder fuzz | Implemented locally | Local decoder fuzz and bounded package tests passed; retain final component evidence against the P01 runtime. |
| T04 observations/last good | `discovery_observation.go`; `DiscoveryWorkAdapter`; `p02_discovery_worker_integration_test.go` | Implemented locally | Atomic success, terminal-failure, and retryable-failure head updates are covered in tagged tests; execute them against a reachable PostgreSQL gate. |
| T05 durable discovery | Q29, active-work index, `DiscoveryWorkAdapter`, `syntheticdiscovery.Executor`, migration 00016, `cmd/discovery-worker` | Implemented locally | Claims are filtered to `DISCOVER`, transport runs outside DB locks, retry evidence/reschedule is atomic, retry timing is attempt-aware (1s then 2s plus deterministic <=250ms jitter), attempts are capped at three, lease repair completes exhausted claims, and a dedicated worker has mandatory CIDR/credential configuration. Fairness barriers and reachable PostgreSQL execution remain open. |
| T06 bounded selectors | `internal/domain/inventory/selector.go`, Q31, `target_selection.go`, `FuzzSelectorCanonicalRoundTrip` | Implemented locally | The 10-second local canonicalization fuzz gate passed. Execute tagged SQL proof and retain the query-plan/large-inventory evidence. |
| T07 frozen preflight | `internal/domain/preflight`, application evaluator, `PreflightAdapter`, PostgreSQL candidate writer, Q32 drift report, `TestP02PreflightCandidatePersistsFrozenMembershipImmutably` | Implemented locally | The tagged Neon application-path test proves selection, verified-artifact loading, fresh discovery evidence, atomic freeze, and retained negative drift/retirement checks. Runnable process composition remains open. |
| T08 readiness recheck | strengthened `AcquireReadinessEnrollment` active/quarantine checks | Partial | P03-owned campaign construction and candidate/artifact/freshness recheck wiring remain open. |
| T09 artifact identity/lifecycle | `internal/domain/artifact`, migrations 00008/00010, immutable guards | Implemented locally | Real PostgreSQL migration/guard execution remains required. |
| T10 direct upload | Q34; artifact upload application/PG adapters; signed artifact-bound grants; durable `localartifactstore`; `localobjecthttp`; `cmd/local-object-store` completion composition | Implemented locally | A process-level listener-to-Neon component run and final retained evidence remain open. |
| T11 verification work | bounded verifier, atomic `UPLOADED -> VERIFYING` plus due work, exact tuple fence; `ArtifactVerificationWorkAdapter`; artifact verification worker; `TestP02ArtifactVerificationWorkerStreamsBoundVersionAndCompletesLease` | Implemented locally | The tagged Neon component test proves claim -> immutable local stream -> `VERIFIED` -> fenced `COMPLETED`; a process-supervision/crash-retry scenario remains open. |
| T12 stable eligibility | immutable verification result, retirement and preflight-state gates | Partial | Readiness transaction must consume these rechecks in the P03 boundary. |
| T13 compatibility | exact synthetic family policy and frozen per-target decision | Implemented locally | Final component proof against all required target profiles remains open. |
| T14 cleanup/orphans | bounded DB expiration; durable cleanup intents with lease fencing; `DiskStore.ReportOrphans`; exact grant receipt resolution; `RunCleanupOnce`; `TestP02CleanupWorkerDeletesOnlyClaimedGrantReceipt` | Implemented locally | The tagged Neon/disk component test proves exact claimed-version deletion and fenced completion. Lease-expiry repair/retry proof remains open. |

## Executed local evidence

- `go test -count=1 ./...` passed at the current branch head.
- `go vet ./...` passed at the current branch head.
- `git diff --check` passed at the current branch head.
- The tagged PostgreSQL package compiles with
  `go test -tags postgresintegration -run '^$' ./test/integration/postgres`.
- Focused discovery application, persistence, and synthetic-executor tests
  passed at `f8edda1`; the tagged package compiled at that SHA. The tagged
  runtime test requires a reachable `FLEETFORGE_TEST_POSTGRES_DSN` and was not
  claimed from the current refused localhost configuration.
- `go test -count=1 ./cmd/local-object-store ./internal/platform/config ./internal/delivery/localobjecthttp ./internal/platform/localartifactstore` passed, including a listener composition test and durable signed-grant receipt recovery after reopening the disk store.
- `TestP02ArtifactUploadCapabilityReservesDatabaseKeyBeforeSigning` passed against a generated isolated Neon schema: durable key reservation, signed artifact-bound capability, direct disk upload, exact streamed verification, and immutable PostgreSQL binding.

## Unexecuted or deferred gates

- The complete tagged PostgreSQL suite has not yet been retained at the final
  integration SHA. Focused P02 artifact lifecycle and direct-upload tests have
  passed against generated isolated Neon schemas.
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
