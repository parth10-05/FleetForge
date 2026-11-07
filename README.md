# FleetForge

FleetForge is a safety-oriented control plane for coordinating approved BMC
firmware rollouts through Redfish. It is not a firmware builder.

The repository is being implemented in gated phases. Read the
[documentation map](markdown.md), the
[core delivery contract](docs/implementation-plan/00-core-principles-and-delivery-contract.md),
and the [implementation-plan index](docs/implementation-plan/README.md)
before changing code.

## Foundation developer mode

Phase 01 requires Go only. It does not require a laptop PostgreSQL instance,
AWS account, external target, firmware image, or cloud credential.

```text
make test
make test-race
make build
make verify
```

Copy `.env.example` only for local API or simulator runs. It contains safe
dummy values; do not place a secret value in an environment file committed to
this repository.

## PostgreSQL developer branch

Phase 03 keeps application traffic and migrations on separate database paths.
The application reads `FLEETFORGE_POSTGRES_APP_DSN` and the migration command
reads `FLEETFORGE_POSTGRES_MIGRATION_DSN`. For a linked Neon development
branch, the standard `DATABASE_URL` (pooled) and `DATABASE_URL_UNPOOLED`
(direct) variables are accepted as their respective fallbacks. Pull those
variables only into an ignored local `.env.local`; do not copy a connection
string into `.env.example`, source control, logs, or issue comments. Migrations
must always use the direct, unpooled URL.

The API exposes only operational endpoints during Phase 01:

- `GET /health/live`
- `GET /health/ready`
- `GET /health/startup`
- `GET /version`

No campaign, target, database, queue, cloud, Redfish, or authentication
behavior exists in this phase.

## Local demo runtime ownership

The P06 launcher creates an owned private run directory, applies migrations
through `DATABASE_URL_UNPOOLED`, then builds and starts the supported local
binaries. Supply the application and direct PostgreSQL URLs only through the
calling environment; the launcher never writes them to an env file. Use a
unique child below `.fleetforge/demo-runs`:

```powershell
$env:DATABASE_URL = 'postgres://...'
$env:DATABASE_URL_UNPOOLED = 'postgres://...'
scripts/demo/run-v1.ps1 -RunRoot .fleetforge/demo-runs/example -Seed example-seed
scripts/demo/stop-v1.ps1 -RunRoot .fleetforge/demo-runs/example
```

The stop script refuses roots outside that directory and requires the demo
ownership marker. It records each binary PID and start time, terminates only a
matching owned process, waits for it to exit, and removes only the owned run.
The launcher derives a private synthetic credential map and simulator run ID
from the selected fixture, then starts only the local object store, simulator,
orchestrator, discovery, wakeup, precheck, execution, observation,
reconciliation, and control-API roles.
It validates a private local admin token with an authenticated bounded API read
and does not print secrets. The run is a local synthetic smoke environment;
hosted acceptance evidence remains the release gate.
