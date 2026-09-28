# Simulator implementation status

This is an evidence index for the repository-owned synthetic target contract.
Entries are complete only to the stated synthetic scope and must be revalidated
on the final integration SHA. It makes no external-device claim.

| Requirement | Current evidence | Status | Limitation / next gate |
| --- | --- | --- | --- |
| T01 resource matrix and fixture provenance | `supported-resource-matrix.md`, `fixture-provenance.md`, `test/fixtures/redfishsim/v1/schema-manifest.json` | Implemented locally | Final revalidation must use the integration SHA; this remains a synthetic contract, not a device claim. |
| T02 versioned bounded scenarios | `internal/simulator/scenario`; strict decoder and five canonical fixtures | Implemented locally | Scenario configuration deliberately remains transport-neutral. |
| T03 logical time and identities | `internal/simulator/clock`, `internal/simulator/core`, `cmd/redfish-simulator` real-time adapter | Implemented locally | The demo adapter maps one wall-clock millisecond to one logical tick. |
| T04 retained state and journal | `internal/simulator/journal`, `internal/platform/simulatorstate`, executable owner/recovery wiring | Implemented locally | Recovery is fail-closed; crash-injection remains an integration test enhancement. |
| T05 Redfish resources and authentication | `internal/delivery/redfishsim`; route and fixture tests | Implemented locally | Only the selected V1 surface is implemented. |
| T06 accepted-update ordering | Typed request/acceptance records in `redfishsim`; journal replay tests | Implemented locally | A request receipt can exist without acceptance when validation rejects it. |
| T07 update/event fault behavior | Transport, observation, lifecycle, and callback dispatcher fault paths | Implemented locally | Fault behavior is synthetic and bounded to the catalog. |
| T08 separate admin boundary | Separate admin listener, independent secret, status/metrics/journal endpoints | Partially implemented | Explicit reset and scenario replacement controls are intentionally not yet exposed. |
| T09 resource limits and readiness | Body/header limits, request concurrency, callback workers/queue, journal-entry cap, post-recovery readiness | Implemented locally | Export size is bounded to journal status rather than raw journal download. |
| T10 safe metrics | Admin-only aggregate request/task/fault counters | Implemented locally | Counters intentionally exclude credentials, destinations, and firmware bytes. |
| T11 canonical scenarios | Healthy, lost-event, post-accept, task-disappearance, and canary-health-failure fixtures | Implemented locally | Later phase acceptance tests consume these saved inputs. |

## Cross-cutting proof already executable

- Scenario decoding rejects duplicate keys, unknown fields, trailing documents,
  unsupported faults, invalid bounds, and invalid topology.
- Logical timers have a defined total order and engine commands receive one
  serialized sequence.
- Journal recovery rejects a corrupt chain, sequence gap, hash mismatch, and
  leading snapshot; a retained run cannot have two owners.
- The public simulator boundary cannot reach the admin status endpoint.
- Architecture tests prohibit simulator imports of FleetForge persistence.
- Full local verification at the current branch head: `go test -count=1 ./...`,
  `go vet ./...`, and all three process builds.

## Remaining scope

The only intentional P01 gap is an authenticated admin reset/scenario
replacement workflow. It is not required for a retained deterministic run and
is kept separate because replacing a scenario must not overwrite a live or
recoverable run. No public Redfish route can invoke it.
