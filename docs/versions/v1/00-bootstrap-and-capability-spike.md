# V1-P00 — Local Bootstrap and Synthetic Target Contract Freeze

## Goal

Create a reproducible local development boundary and freeze the repository-owned
synthetic target contract before adapter and route behavior are written. Keep
the AWS infrastructure definition available for later deployment, but require
no AWS account, live cloud resource, external firmware image, emulator, or
specialized hardware to build and verify v1.

## Tasks

- [ ] **V1-P00-T01** Record the local toolchain, process ports, retained-run directories, resource ceilings, reset procedure, and evidence paths. A clean developer or CI runner must reproduce the boundary without cloud credentials.
- [ ] **V1-P00-T02** Keep the Terraform foundation plan-only, default-off, and zero-scale. Validate formatting, tests, policy checks, naming, ownership tags, expiry rules, and forbidden-resource rules without running `apply` or creating an AWS identity.
- [ ] **V1-P00-T03** Define provider-neutral ports for versioned object storage, queue wakeups, credential resolution, and OIDC identity. Supply deterministic local test adapters that model only the semantics used by v1; keep AWS SDK types outside domain and application contracts.
- [ ] **V1-P00-T04** Run PostgreSQL integration tests against an ephemeral real PostgreSQL service in CI. Treat Neon pooled/direct connectivity as a later provider gate rather than a prerequisite for local implementation.
- [ ] **V1-P00-T05** Freeze a local test-identity contract and Cognito-compatible OIDC configuration shape. Live Cognito federation, issuer discovery, and hosted login evidence belong to the AWS staging bootstrap.
- [ ] **V1-P00-T06** Define the local/containerized synthetic target boundary, separate target/admin credentials and listeners, retained-run ownership, callback allowlist, and absence of external target routes.
- [ ] **V1-P00-T07** Pin one official DMTF release plus the exact selected schema files, source URLs, checksums, retrieval date, and fixture license.
- [ ] **V1-P00-T08** Freeze canonical Service Root, identity, UpdateService, TaskService, FirmwareInventory, EventService, authentication, and bounded error fixtures for the repository-owned backend.
- [ ] **V1-P00-T09** Freeze one synthetic `HttpPushUri` contract with streamed synthetic artifact bytes, digest/version binding, durable acceptance ordering, task, reboot, inventory, health, failure, and recovery behavior.
- [ ] **V1-P00-T10** Accept ADR-032, ADR-038, and ADR-039. Stop before route implementation if any behavior, extension, limit, or evidence oracle remains implicit.
- [ ] **V1-P00-T11** Add CI checks for IaC formatting, validation, tests, and security policy while proving the workflow has no AWS credentials, OIDC token permission, provider apply, or tracked target credential plaintext.
- [ ] **V1-P00-T12** Produce a local bootstrap record containing tool versions, zero-resource IaC proof, provider-port contracts, fixture provenance, synthetic-target reset/recovery procedure, and an explicit handoff listing the live AWS checks deferred to v2.

## Required proof

- A clean local or hosted CI runner completes the v1 fast gate without an AWS account, cloud credential, external device, firmware image, or emulator.
- The default IaC configuration plans zero runtime resources and the pull-request workflow cannot obtain an AWS identity.
- Deterministic local adapters prove the application contracts for version identity, duplicate/lost wakeups, credential-reference resolution, and OIDC claims without being presented as AWS provider evidence.
- The synthetic target fixtures are sufficient to implement one exact update path with no unstated route behavior, or v1 stops.

## Exit gate

The local environment and contract fixtures are reproducible, all provider
boundaries are explicit, and the synthetic target backend runs without cloud
resources. The AWS IaC remains available for a later reviewed deployment, but
no v1 implementation task may require a live AWS resource unless the project
owner explicitly reopens that gate.
