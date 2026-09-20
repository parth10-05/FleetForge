# V2-P03 — Operator Console

## Goal

Build the browser operator surface as a bounded projection and command client. It must explain uncertainty and exposure rather than turn the control plane into optimistic CRUD.

## Tasks

- [ ] **V2-P03-T01** Establish authenticated routing, role-aware navigation, build/environment identity, error/stale/loading states, and accessible semantic state tokens.
- [ ] **V2-P03-T02** Build shared revision, freshness, reason-code, risk-callout, confirmation-dialog, pagination, filtering, timeline, and chart primitives with non-color equivalents.
- [ ] **V2-P03-T03** Implement target list/detail/registration/disable/refresh surfaces with bounded filters, keyset pagination, safe observations, administrative permission checks, and no credential exposure.
- [ ] **V2-P03-T04** Implement artifact direct-upload initiation/progress/status and evidence display without retaining signed URLs or object credentials in application state.
- [ ] **V2-P03-T05** Build a stepwise campaign editor for selector, artifact, canary, waves, budgets, limits, health, and maintenance windows; client validation improves usability but server policy remains authoritative.
- [ ] **V2-P03-T06** Render preflight and immutable plan review with warnings, acknowledgement, snapshot hash, exact target assignments, effective threshold math, and failure-domain distribution.
- [ ] **V2-P03-T07** Build the active control room: lifecycle, target distribution, physical reservations, admitted exposure, budget, overdue work, reconciliation lag, and accurate pause/resume/abort semantics.
- [ ] **V2-P03-T08** Integrate SSE/reconnect/gap refetch with bounded cache updates and explicit disconnected/stale mode. A browser event never becomes workflow truth.
- [ ] **V2-P03-T09** Build the attention queue/evidence chronology/admin resolution flow and immutable audit timeline. Require recent-auth, reason, revision, idempotency, and explicit reservation disposition.
- [ ] **V2-P03-T10** Complete browser E2E, accessibility, keyboard-only, slow/offline/reconnect, XSS-shaped content, stale-revision, and 1,000-target rendering-profile tests without claiming backend load validation.

## Exit gate

The console lets authorized operators complete the documented workflow without direct API tools, accurately distinguishes device observation from controller truth, and never hides `NEEDS_ATTENTION`, stale evidence, or pre-pause admitted exposure.
