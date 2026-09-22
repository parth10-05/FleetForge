# ADR 013: Infrastructure state and federated delivery

## Status

Accepted for code preparation; administrative execution is pending.

## Decision

- Environment composition uses Terraform with reusable modules and default-off
  feature flags.
- Remote state configuration is supplied only from an uncommitted backend
  configuration during the administrative setup; state must be encrypted,
  access-controlled, recoverable, and owned by a named operator.
- Pull-request verification has no cloud credentials or OIDC token permission.
- Apply and teardown are manual workflows behind distinct protected GitHub
  Environments and use GitHub OIDC, not long-lived AWS access keys.

## Consequences

Terraform modules do not prove the state backend, IAM trust, or provider
access. Those claims require dated administrative evidence in the bootstrap
exit report.
