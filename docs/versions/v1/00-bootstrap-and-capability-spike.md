# V1-P00 — AWS Development Bootstrap and Virtual-BMC Contract Freeze

## Goal

Create the smallest reproducible AWS development boundary required by later v1
application services and freeze one repository-owned virtual-BMC update
contract before adapter and route behavior are written. The backend itself
must remain runnable locally without AWS.

## Tasks

- [ ] **V1-P00-T01** Create a disposable AWS development environment and record region, owner, expiration, monthly budget alert, teardown owner, and maximum resource counts. Do not create a public production environment.
- [ ] **V1-P00-T02** Provision least-privilege AWS IAM roles for API, worker, migration, simulator administration, and GitHub Actions OIDC. Prove an untrusted pull request cannot obtain cloud credentials.
- [ ] **V1-P00-T03** Provision a private versioned S3 bucket, SQS Standard queue(s), EventBridge Scheduler bounded publisher/sweeper task, Secrets Manager placeholders, ECR repository, and minimum-zero ECS API/worker shells through reviewed IaC.
- [ ] **V1-P00-T04** Create the disposable Neon development project and separate application, direct-migration, and diagnostic roles. Store DSNs only through Secrets Manager injection; prove pooled application and direct migration paths separately.
- [ ] **V1-P00-T05** Configure one Cognito User Pool with Google federation and the callback/issuer/audience values used by the v1 API. Do not implement custom password storage or expose client secrets.
- [ ] **V1-P00-T06** Define the local/containerized simulator boundary,
  separate Redfish/admin credentials and listeners, retained-run ownership,
  callback allowlist, and explicit absence of external target routes.
- [ ] **V1-P00-T07** Pin one official DMTF release plus the exact selected
  schema files, source URLs, checksums, retrieval date, and fixture license.
- [ ] **V1-P00-T08** Freeze canonical Service Root, identity, UpdateService,
  TaskService, FirmwareInventory, EventService, authentication, and bounded
  error fixtures for the repository-owned backend.
- [ ] **V1-P00-T09** Freeze one synthetic `HttpPushUri` contract with streamed
  artifact bytes, digest/version binding, durable acceptance ordering, task,
  reboot, inventory, health, failure, and recovery behavior.
- [ ] **V1-P00-T10** Accept ADR-032, ADR-038, and ADR-039. Stop before route
  implementation if any behavior, extension, limit, or evidence oracle remains
  implicit.
- [ ] **V1-P00-T11** Add CI checks for IaC formatting/validation/security scan and ensure no Terraform state or GitHub configuration contains BMC credential plaintext.
- [ ] **V1-P00-T12** Produce the bootstrap exit report: IAM matrix, cost ledger,
  resource inventory, Neon connection budget, virtual-BMC contract fixture,
  local shutdown/reset procedure, and cloud teardown runbook.

## Required proof

- `P13-B03` queue repair works while worker desired capacity is zero.
- `P13-B04` proves pooled/direct Neon configuration without treating it as a Phase 03 claim.
- `P13-IAM-01`, `P13-IAM-02`, `P13-DP-06`, and a disposable teardown/recreate smoke pass.
- The contract fixture is sufficient to implement one exact synthetic update
  path with no unstated route behavior, or v1 stops.

## Exit gate

All provider credentials are short-lived or reference-only, the disposable
environment has enforceable caps and a teardown path, and the virtual-BMC
contract is frozen with canonical fixtures and explicit limitations. The
simulator runs locally without cloud resources; do not implement routes from
unstated assumptions.
