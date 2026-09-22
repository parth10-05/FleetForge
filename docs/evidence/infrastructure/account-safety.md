# Account safety controls

Infrastructure changes begin only after an account owner records the account
plan state, credit/expiry threshold, approved region, emergency teardown owner,
and confirmation that the account is outside Organizations and Control Tower.

The FleetForge operator identity must use MFA and temporary role sessions. It
must have no access key. Existing shared-account administrator policies are not
changed by this repository; an account owner reviews them separately.

Before any apply, the owner must confirm a billing alert address, current usage,
and a maximum session duration. The preflight command fails closed for inactive
or near-expiry plans, a non-approved region, or a prefixed resource owned by a
different operator.
