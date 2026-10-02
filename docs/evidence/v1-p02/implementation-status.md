# V1-P02 implementation status

This is a source-and-evidence index for `V1-P02 — Target Inventory,
Preflight, and Artifact Trust`. It records the current local implementation
state on `codex/v1-p02-inventory-artifact-trust`; it is not a phase-closeout
claim and makes no cloud-provider, external-device, or firmware-submission
claim.

| P02 task | Current evidence | Status | Remaining gate |
| --- | --- | --- | --- |
| T01 registration | `internal/application/inventory`, `internal/domain/inventory`, `internal/persistence/postgres/registration.go`, `cmd/v1-p02-check` | Verified on isolated PostgreSQL | The readiness command rejects a database below required P02 migration 16 while allowing later schemas. The complete tagged PostgreSQL suite passed against the configured isolated direct Neon connection; runnable command composition remains an operational integration concern. |
| T02 endpoint protections | `internal/redfish/address.go`, `client.go`, address tests and `FuzzTargetAddressKeepsAcceptedResourcesUnderServiceRoot` | Implemented locally | The 10-second local address fuzz gate retained no service-root escape. The P02 executor completed an in-process P01 simulator route flow under the loopback CIDR policy; deployed-policy evidence remains open. |
| T03 Redfish discovery bounds/errors | `internal/redfish/{client,decode,discovery,errors}.go`, synthetic integration test, decoder fuzz, `TestExecutorDiscoversAgainstP01SyntheticRuntime` | Implemented locally | Local decoder fuzz, bounded package tests, and an authenticated executor-to-P01-runtime component flow passed; retain final deployed-runtime evidence. |
| T04 observations/last good | `discovery_observation.go`; `DiscoveryWorkAdapter`; `p02_discovery_worker_integration_test.go` | Verified on isolated PostgreSQL | Atomic success, terminal-failure, and retryable-failure head updates passed in the complete tagged suite. |
| T05 durable discovery | Q29, active-work index, `DiscoveryWorkAdapter`, `syntheticdiscovery.Executor`, migration 00016, `cmd/discovery-worker`, `TestP02DiscoveryClaimsOldestDueWorkAndSkipsExistingLease` | Verified on isolated PostgreSQL | Claims are filtered to `DISCOVER`, transport runs outside DB locks, retry evidence/reschedule is atomic, retry timing is attempt-aware (1s then 2s plus deterministic <=250ms jitter), attempts are capped at three, lease repair completes exhausted claims, and a dedicated worker has mandatory CIDR/credential configuration. Process supervision remains an operational integration concern. |
| T06 bounded selectors | `internal/domain/inventory/selector.go`, Q31, `target_selection.go`, `FuzzSelectorCanonicalRoundTrip`, `TestP02TargetSelectionFailsClosedAtTenThousandCandidates` | Verified on isolated PostgreSQL | The 10-second canonicalization fuzz gate passed, and the complete tagged suite exercised the 10,000-target fail-closed cap plus retained exact-label index schema evidence. This does not assert a universal PostgreSQL planner choice. |
| T07 frozen preflight | `internal/domain/preflight`, application evaluator, `PreflightAdapter`, PostgreSQL candidate writer, Q32 drift report, `TestP02PreflightCandidatePersistsFrozenMembershipImmutably` | Implemented locally | The tagged Neon application-path test proves selection, verified-artifact loading, fresh discovery evidence, atomic freeze, and retained negative drift/retirement checks. Runnable process composition remains open. |
| T08 readiness recheck | `AcquireReadinessEnrollment` serializable trust boundary; candidate/artifact/object-version/fresh-evidence checks; `TestReadinessEnrollmentRejectsInvalidatedTrustInputsAtomically`; `TestReadinessEnrollmentRollsBackEarlierClaimsWhenALaterMemberIsBlocked` | Implemented locally | The readiness handoff is atomic and rechecks frozen candidate membership, exact verified artifact tuple, current target revision/trust metadata, matching discovery head, freshness, compatibility, existing enrollment, and reservations. P03 still owns policy/member construction and may not bypass this boundary. |
| T09 artifact identity/lifecycle | `internal/domain/artifact`, migrations 00008/00010, immutable guards | Verified on isolated PostgreSQL | The complete tagged suite applied the migrations and exercised lifecycle/constraint guards. |
| T10 direct upload | Q34; artifact upload application/PG adapters; signed artifact-bound grants; durable `localartifactstore`; `localobjecthttp`; `cmd/local-object-store` completion composition | Verified component path | The six-target component test exercised PostgreSQL-backed artifact registration, an independent signed local-object HTTP listener, exact bind, and verification. This is local-provider proof only, not S3/provider proof. |
| T11 verification work | bounded verifier, atomic `UPLOADED -> VERIFYING` plus due work, exact tuple fence; `ArtifactVerificationWorkAdapter`; artifact verification worker; `TestP02ArtifactVerificationWorkerStreamsBoundVersionAndCompletesLease`; `TestVerifyContentStreamsMaximumArtifactWithoutLargeReadBuffer` | Implemented locally | The verifier's 16 MiB stream test retains no source blob and observes no read request larger than 64 KiB. Tagged source coverage proves claim -> immutable local stream -> `VERIFIED` -> fenced `COMPLETED`; process-supervision/crash-retry and reachable PostgreSQL execution remain open. |
| T12 stable eligibility | immutable verification/retirement gates plus the T08 atomic recheck transaction | Implemented locally | Retirement, missing candidate, and target-revision drift are rejected before enrollment, campaign transition, audit, or outbox writes. Broader component and crash-replay evidence remains open. |
| T13 compatibility | exact synthetic family policy and frozen per-target decision | Implemented locally | Final component proof against all required target profiles remains open. |
| T14 cleanup/orphans | bounded DB expiration; durable cleanup intents with lease fencing; `DiskStore.ReportOrphans`; exact grant receipt resolution; `RunCleanupOnce`; `TestP02CleanupWorkerDeletesOnlyClaimedGrantReceipt`; stale-lease recovery assertion in `TestP02CleanupExpiresOnlyOldUnboundUploadingArtifacts` | Verified component path | The complete tagged suite and six-target component flow prove exact claimed-version deletion, fenced completion, stale-token rejection after lease recovery, and retention of the verified object. |

## Executed local evidence

- `go test -count=1 ./...` passed at the current branch head.
- `go vet ./...` passed at the current branch head.
- `git diff --check` passed at the current branch head.
- GitHub Actions run [`36916288205`](https://github.com/parth10-05/FleetForge/actions/runs/36916288205) passed at `39d5f412d00dbbb701a191390a577715dec82f3a`: SQLC drift, formatting, vet, unit/property, complete tagged PostgreSQL, normal and tagged Linux race, all three fuzz gates, command builds, web, changed-files, and secret scanning.
- `go test -count=1 -tags postgresintegration ./test/integration/postgres/...` passed against the configured direct Neon connection. The dotenv file encloses the direct URL in quotes; the test process removes only those outer quotes before assigning `FLEETFORGE_TEST_POSTGRES_DSN`, without writing or exposing the credential.
- `go test -count=1 -tags postgresintegration -run '^TestReadinessEnrollment' ./test/integration/postgres` passed against the same isolated connection.
- The three bounded fuzz gates passed locally: `FuzzSelectorCanonicalRoundTrip`, `FuzzTargetAddressKeepsAcceptedResourcesUnderServiceRoot`, and `FuzzDecodeDocumentRejectsUnsafeInputWithoutPanic`.
- Local `go test -race` cannot compile because the installed Windows C compiler reports `64-bit mode not compiled in`; this is not a code-test result. The exact-SHA Linux CI run above is the authoritative normal and tagged race evidence.
- `go test -count=1 ./cmd/local-object-store ./internal/platform/config ./internal/delivery/localobjecthttp ./internal/platform/localartifactstore` passed, including a listener composition test and durable signed-grant receipt recovery after reopening the disk store.
- `TestP02ArtifactUploadCapabilityReservesDatabaseKeyBeforeSigning` passed against a generated isolated Neon schema: durable key reservation, signed artifact-bound capability, direct disk upload, exact streamed verification, and immutable PostgreSQL binding.

## Unexecuted or deferred gates

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
