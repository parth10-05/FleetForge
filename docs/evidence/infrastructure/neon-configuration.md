# Neon configuration evidence

This repository does not store a Neon API key, project identifier, connection
string, or database password. Use `scripts/infra/configure-neon.ps1` only from
a controlled administrative session.

Record, after the administrative run:

- environment and project identifier redacted to the approved evidence form;
- separate application, migration, and read-only role creation outcome;
- whether pooled and direct endpoints were configured for their intended use;
- configured compute, retention, and session-stop controls; and
- evidence location for secrets-manager injection, excluding all DSNs.

Do not claim a provider configuration or recovery window until a dated,
provider-backed administrative record exists.
