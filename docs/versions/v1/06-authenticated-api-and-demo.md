# V1-P06 — Authenticated API and Reproducible Demo

## Goal

Expose only the operator flows required for the v1 demo through a versioned, Cognito-authenticated API and a repeatable CLI/script. This is a thin Phase 10 slice, not completion of the full API or browser console.

## Tasks

- [ ] **V1-P06-T01** Publish a minimal OpenAPI 3.1 contract and lint it in CI. Include health/version, targets/discovery, artifact initiate/status, campaign create/validate/start/pause, campaign status/audit, and attention resolution endpoints.
- [ ] **V1-P06-T02** Implement isolated Cognito OIDC validation for issuer, audience, signature, expiry, `auth_time`, and role mapping. Keep provider types/tokens outside domain/application rules and redact authentication errors.
- [ ] **V1-P06-T03** Enforce deny-by-default roles: viewer reads, operator performs draft/validate/start/pause, admin registers targets/artifacts and resolves attention. Prevent resource existence leaks.
- [ ] **V1-P06-T04** Require `Idempotency-Key` for every mutation and expected revision where a state race is possible. Exact duplicates replay a stable result; same key/different request returns a safe conflict.
- [ ] **V1-P06-T05** Map each handler to exactly one application command/query. No handler may run Redfish I/O, transition state directly, or wait for firmware completion; asynchronous actions return a safe operation/resource reference.
- [ ] **V1-P06-T06** Apply explicit body/header/query/page/time limits, safe RFC 9457 problem details, correlation IDs, and route-level rate limits for high-risk commands.
- [ ] **V1-P06-T07** Return bounded target/campaign/audit projections with revision and `as_of` time. Do not add SSE or browser-specific projections in v1.
- [ ] **V1-P06-T08** Ensure successful mutations produce same-transaction lifecycle audit evidence and denied high-risk attempts produce safe audit metadata where required.
- [ ] **V1-P06-T09** Create a seed-driven CLI/demo script that obtains a test identity, provisions only synthetic credential references, runs the v1 demonstration, exports redacted audit/journal evidence, and tears down synthetic state safely.
- [ ] **V1-P06-T10** Add a v1 evidence record with commit SHA, scenario seed, exact API/image/schema/IaC versions, hosted CI links, test outputs, cloud cost snapshot, and limitations.
- [ ] **V1-P06-T11** Add README instructions for an independent developer to
  run the safe local simulator demo; do not publish credentials, signed URLs,
  admin secrets, or retained-run control paths.

## Required proof

- Invalid/expired/wrong-audience identities cannot mutate; every role/action decision follows the v1 matrix.
- Duplicate/stale/high-risk commands cannot create a second transition or bypass recent-auth/manual-resolution controls.
- Error/log fuzzing shows no token, DSN, signed URL, BMC credential, firmware byte, or raw device payload leakage.
- A clean synthetic environment completes healthy, lost-event, post-accept ambiguity, and canary-stop demonstrations using documented commands only.

## v1 release gate

All v1 phase gates and applicable CI tests pass. The release statement says
`virtual-BMC-backed v1 demonstration` and links to retained evidence. It must
not claim browser-console completeness, production deployment, 1,000-target
performance, external-device compatibility, or vendor support.
