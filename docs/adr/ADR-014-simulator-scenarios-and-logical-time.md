# ADR-014 — Simulator scenarios and logical time

## Status

Accepted for the deterministic synthetic target backend and its synthetic
Redfish-shaped update route.

## Context

FleetForge needs a simulator that can expose repeated firmware submissions and
faults without hiding controller mistakes. Wall-clock timers, random IDs, and
unstructured test setup cannot provide a replayable independent journal.

## Decision

- Simulator scenarios use strict, versioned JSON with a declared seed,
  synthetic target topology, bounded timing, and an ordered fault schedule.
- The simulator uses injected logical time in tests. A real-time adapter may
  translate wall time into the same commands for an interactive demonstration.
- Run, target, request, accepted-operation, task, and event identifiers are
  deterministically derived from scenario identity, seed, kind, and ordinal.
- A single engine owns mutable virtual-device truth. HTTP handlers and callback
  completions submit immutable commands; they do not mutate target state.
- Scenario files, journals, metrics, errors, and fixtures exclude credentials,
  firmware bytes, signed URLs, and real endpoints.

## Consequences

The simulator can make a reproducibility claim only for a saved scenario,
seed, initial state, and ordered command trace. It does not claim deterministic
ordering for unspecified simultaneous network arrivals, full Redfish
conformance, vendor behavior, or external-device behavior.

ADR-039 and ADR-032 freeze the accepted synthetic update route. No simulator
handler may invent behavior outside that contract and its supported-resource
matrix.
