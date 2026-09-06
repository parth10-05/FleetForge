# Phase 01 container baseline

Each Go role has a separate multi-stage image and runs as the distroless
non-root user. The API and worker require no writable root filesystem. The
simulator requires an explicit writable mount for its state and journal paths;
it must never use an undeclared ephemeral path for retained test evidence.

The Dockerfiles intentionally omit a Docker `HEALTHCHECK`: ECS health semantics
will be configured with the platform in Phase 13, and the application already
exposes explicit live/readiness/startup endpoints. A container-health command
that cannot express readiness correctly would be misleading.
