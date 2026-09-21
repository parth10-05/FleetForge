# FleetForge infrastructure

This directory begins the **plan-only Terraform foundation**. It is safe to
review and validate without AWS access. The default `v1-dev` root enables no
runtime feature and declares no customer-managed AWS resources.

## Safety boundary

- Do not run `terraform apply`, `terraform destroy`, import, or state commands
  from this repository until a later reviewed run explicitly permits it.
- Do not put account IDs, DSNs, OAuth secrets, access keys, session tokens,
  backend configuration, state, binary plans, or live evidence in Git.
- The pull-request workflow has read-only repository permission only. It has
  no AWS credentials, GitHub OIDC token, environment access, apply, or destroy
  step.
- Terraform provider/resource modules are deliberately deferred. Run 02 owns
  state and GitHub OIDC; resource modules begin only in their assigned gated
  runs.

## Local static validation

Once the pinned tools are installed, run from
`infra/environments/v1-dev`:

```powershell
terraform fmt -check -recursive ../../..
terraform init -backend=false
terraform validate
terraform test
```

Use `scripts/infra/preflight.ps1 -FixturePath <fixture>` only with synthetic
fixtures in this run. It must not receive real account data and it does not
call AWS.

## Configuration contract

All future resources use the `ps-fleetforge-v1-dev-` naming contract and the
required ownership tags described in [naming.md](naming.md). Feature flags are
off by default; enabling one requires a future apply-specific expiry record.

This foundation does not prove AWS permissions, remote state, billing, live resources,
or QEMU behavior. Those claims require dedicated identity, cost, and provider
evidence in later provider-gated work.
