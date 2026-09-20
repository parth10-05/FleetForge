# V2-P00 — Entry Gate and Staging Boundary

## Goal

Promote the v1 simulator demonstration into a controlled staging baseline without inflating its claims or mixing unfinished v1 safety work with new operator features.

## Tasks

- [ ] **V2-P00-T01** Close every v1 task or record its explicit deferral, risk, owner, and version destination in the Decision Log, Traceability Matrix, and Risk Register.
- [ ] **V2-P00-T02** Tag the last v1 evidence commit and retain its scenarios, simulator journals, audit exports, image digests, schema version, IaC revision, tool versions, and cost record.
- [ ] **V2-P00-T03** Create/review a staging environment separate from disposable development: distinct Neon roles/database, S3 namespace, queues, Cognito app configuration, Secrets Manager references, simulator run store, and cost caps.
- [ ] **V2-P00-T04** Enforce GitHub Actions OIDC claims, protected environment rules, environment-specific configuration validation, and no-long-lived cloud credential policy.
- [ ] **V2-P00-T05** Build and publish immutable API, worker, and simulator image digests from CI; record SBOM, vulnerability, license, and provenance inputs before deployment.
- [ ] **V2-P00-T06** Deploy only synthetic targets/artifacts to staging and prove test/demo routes cannot reach a real BMC endpoint or live credential reference.
- [ ] **V2-P00-T07** Run a baseline healthy and post-accept-ambiguity v1 scenario in staging. Record environment, seed, time window, durable evidence, cost, and known limitations.
- [ ] **V2-P00-T08** Define v2 compatibility policy for OpenAPI, database expand/migrate/contract, queue envelope versions, event schemas, and simulator scenario versions.
- [ ] **V2-P00-T09** Add a staging reset/cleanup path that protects audit evidence and referenced object versions while deleting only synthetic test state.

## Exit gate

The staging baseline runs the known v1 safety scenarios from immutable artifacts and separates synthetic-only resources from any future real-target boundary. Do not begin new console features while staging still depends on manual schema edits, permanent developer credentials, or undocumented resets.
