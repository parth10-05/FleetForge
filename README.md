# FleetForge

FleetForge is a safety-oriented control plane for coordinating approved BMC
firmware rollouts through Redfish. It is not a firmware builder.

The repository is being implemented in gated phases. Read the
[documentation map](markdown.md), the
[core delivery contract](docs/implementation-plan/00-core-principles-and-delivery-contract.md),
and the [Phase 01 execution plan](docs/implementation-plan/phase-01-detailed-execution-plan.md)
before changing code.

## Foundation developer mode

Phase 01 requires Go only. It does not require a laptop PostgreSQL instance,
AWS account, BMC, firmware image, or cloud credential.

```text
make test
make test-race
make build
make verify
```

Copy `.env.example` only for local API or simulator runs. It contains safe
dummy values; do not place a secret value in an environment file committed to
this repository.

The API exposes only operational endpoints during Phase 01:

- `GET /health/live`
- `GET /health/ready`
- `GET /health/startup`
- `GET /version`

No campaign, target, database, queue, cloud, Redfish, or authentication
behavior exists in this phase.
