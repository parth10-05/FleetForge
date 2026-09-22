# ADR 029: External secret handling

## Status

Accepted.

## Decision

OAuth client secrets, database DSNs, BMC credentials, and access tokens never
enter Terraform variables, tfvars, committed source, plan artifacts, or
evidence. Terraform creates only secret-free resource foundations where the
provider supports that separation. Credentialed administrative scripts accept
required secrets solely from process environment variables.

## Consequences

Google federation and Neon connection injection require separate
administrative evidence. A sensitive Terraform variable is not an acceptable
substitute for this boundary because it can still be represented in state.
