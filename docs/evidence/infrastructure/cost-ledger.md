# Infrastructure cost ledger

Use this ledger only after checking current provider pricing from the official
source on the execution date. Do not treat this template as a pricing claim.

| Date checked | Provider/service | Region | Planned bound | Source URL | Owner | Decision |
|---|---|---|---|---|---|---|
| Pending administrative run | AWS S3/SQS/ECR | us-east-1 | Artifact lifecycle, one queue class, immutable image retention | Pending | ps | Not yet approved |
| Pending administrative run | EC2 QEMU host | us-east-1 | Maximum one stopped/running synthetic simulator | Pending | ps | Not yet approved |
| Pending administrative run | Neon | Provider-selected | Session development or retained environment tier | Pending | ps | Not yet approved |

Before apply, record budget alarms, resource-expiry time, maximum instance
count, queue bounds, artifact lifecycle, registry retention, and the explicit
teardown owner. Update the row with a dated official pricing source rather
than copying an estimate into this repository.
