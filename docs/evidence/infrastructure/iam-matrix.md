# IAM matrix template

This foundation creates no IAM principal, policy, OIDC provider, or credential. This
matrix is populated only after an administrator has created reviewed,
FleetForge-specific identities in a later run.

| Principal | Intended capability | Explicit denial evidence | Status |
| --- | --- | --- | --- |
| Untrusted pull request | Static CI only | No id-token or cloud credential | Implemented in static CI |
