# V1-P01 final integration contract

This record pins the P01 route and recovery evidence consumed by V1-P02. It
does not claim cloud-provider behavior, external-device interoperability, or
firmware submission.

## Pinned evidence

- **Source SHA:** `84703000a625936aa1041dbd4f6352599534e167`
- **Branch:** `codex/v1-p02-inventory-artifact-trust`
- **Linux CI:** [Foundation verification #77](https://github.com/parth10-05/FleetForge/actions/runs/37013995287)
- **Result:** success: changes, secret scan, Go, and web jobs all passed.
- **Go artifact:** `go-test-results`, SHA-256
  `2a6d76c3b5ff34f5019fdb416e6e0ff890e5c18b767bf23d1bc702ec17720aac`.

The Go job executes formatting and SQLC drift checks, unit and domain-model
tests, complete `postgresintegration` coverage, normal and tagged Linux race
tests, the three bounded fuzz gates, and command builds. The job result is the
authoritative Linux evidence; local Windows race compilation is not used as a
substitute.

## P01 contract surface

| Required boundary | Executable evidence at the pinned SHA |
| --- | --- |
| Frozen subset and fixture provenance | `supported-resource-matrix.md`, `fixture-provenance.md`, ADR-032/ADR-039, and the saved deterministic scenario fixtures. |
| Authenticated bounded route | `internal/delivery/redfishsim/router_test.go` constructs `NewWithJournal(...).Routes()` and exercises Basic authentication and route behavior. |
| Independent journal and duplicates | `internal/simulator/journal`, `internal/simulator/core/recovery_test.go`, and engine tests retain accepted requests and make duplicate acceptance visible. |
| Post-accept response loss and replay | `router_test.go` uses `post-accept-disconnect.json`, asserts the lost response, and replays the durable journal record. |
| Single-owner recovery and admin isolation | ADR-038 plus core recovery and simulator-admin tests define and execute the restart/ownership boundary. |
| P02 consumption of frozen subset | `internal/redfish/discovery.go`, `internal/platform/syntheticdiscovery`, and `TestExecutorDiscoversAgainstP01SyntheticRuntime` exercise the P02 adapter against the P01 synthetic runtime. |

## Scope boundary

This artifact is a dependency contract for P02’s read-only discovery evidence.
It does not authorize P03 activation, campaign waves, firmware submission, or
any AWS, S3, Neon-provider, or external BMC claim.
