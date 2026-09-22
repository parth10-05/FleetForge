# V1 development naming and ownership contract

Every FleetForge-owned AWS resource must begin with
`ps-fleetforge-v1-dev-` where the service permits it. A constrained service
name may shorten the suffix only; it must retain `ps-` first and be documented
with its deterministic derivation before an apply is approved.

Required tags are supplied through the AWS provider default tags and repeated
by resources which do not inherit them:

| Tag | Value |
| --- | --- |
| `Project` | `FleetForge` |
| `Owner` | validated owner input |
| `Environment` | `v1-dev` |
| `ManagedBy` | `terraform` |
| `CostScope` | `ps-fleetforge` |
| `DataClass` | `synthetic-only` |
| `VersionScope` | `v1` |
| `ExpiresAt` | approved apply expiry in ISO-8601 UTC |

No destroy is approved unless every target has the required prefix or all
required ownership tags. The prefix improves ownership visibility only; it
does not isolate shared-account billing.
