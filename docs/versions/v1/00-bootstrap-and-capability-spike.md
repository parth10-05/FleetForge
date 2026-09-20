# V1-P00 — AWS Development Bootstrap and QEMU Capability Spike

## Goal

Create the smallest reproducible AWS development boundary required by v1 and freeze one QEMU/OpenBMC update contract before adapter and simulator behavior are written.

## Tasks

- [ ] **V1-P00-T01** Create a disposable AWS development environment and record region, owner, expiration, monthly budget alert, teardown owner, and maximum resource counts. Do not create a public production environment.
- [ ] **V1-P00-T02** Provision least-privilege AWS IAM roles for API, worker, migration, simulator administration, and GitHub Actions OIDC. Prove an untrusted pull request cannot obtain cloud credentials.
- [ ] **V1-P00-T03** Provision a private versioned S3 bucket, SQS Standard queue(s), EventBridge Scheduler bounded publisher/sweeper task, Secrets Manager placeholders, ECR repository, and minimum-zero ECS API/worker shells through reviewed IaC.
- [ ] **V1-P00-T04** Create the disposable Neon development project and separate application, direct-migration, and diagnostic roles. Store DSNs only through Secrets Manager injection; prove pooled application and direct migration paths separately.
- [ ] **V1-P00-T05** Configure one Cognito User Pool with Google federation and the callback/issuer/audience values used by the v1 API. Do not implement custom password storage or expose client secrets.
- [ ] **V1-P00-T06** Provision an isolated on-demand EC2 VM boundary for the simulator and QEMU capability spike. Simulator and QEMU must not share real-target credentials or routes.
- [ ] **V1-P00-T07** Obtain the pinned prebuilt QEMU/OpenBMC image with source, checksum, license/provenance, supported Redfish version, credential/trust procedure, and recovery procedure recorded.
- [ ] **V1-P00-T08** Probe the selected QEMU image for service root, UpdateService, TaskService, SoftwareInventory, authentication, task lifecycle, reboot behavior, and the exact artifact-transfer/update mechanism.
- [ ] **V1-P00-T09** Perform the QEMU update-path spike only with the documented reversible/test image and explicit approval. Record upload/data path, apply time, task/inventory evidence, reboot timeline, failure behavior, and recovery result.
- [ ] **V1-P00-T10** Write/accept ADR-013 and ADR-032 records that freeze the one supported v1 update contract. If the image cannot support a safely recoverable update, stop before Phase 04 and revise the v1 scope.
- [ ] **V1-P00-T11** Add CI checks for IaC formatting/validation/security scan and ensure no Terraform state or GitHub configuration contains BMC credential plaintext.
- [ ] **V1-P00-T12** Produce the bootstrap exit report: IAM matrix, cost ledger, resource inventory, Neon connection budget, QEMU capability fixture, and explicit teardown command/runbook.

## Required proof

- `P13-B03` queue repair works while worker desired capacity is zero.
- `P13-B04` proves pooled/direct Neon configuration without treating it as a Phase 03 claim.
- `P13-IAM-01`, `P13-IAM-02`, `P13-DP-06`, and a disposable teardown/recreate smoke pass.
- The capability fixture is sufficient to implement one exact supported update path, or v1 stops.

## Exit gate

All provider credentials are short-lived or reference-only, the disposable environment has enforceable caps and a teardown path, and the QEMU spike has either frozen a safe v1 contract or documented a blocker. Do not begin simulator emulation from an assumption.
