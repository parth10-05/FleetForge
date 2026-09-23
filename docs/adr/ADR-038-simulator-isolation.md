# ADR-038 — Virtual-BMC isolation and single-owner runtime

## Status

Accepted.

## Decision

The virtual-BMC backend runs locally or as an explicitly deployed simulator
role. Local development binds its Redfish-shaped, admin, and health listeners
to loopback by default. A deployed simulator uses a separate service identity,
network policy, state location, and credentials from the API and worker roles.
It cannot receive external-target credentials or unrestricted destination
routes.

A retained simulator run is owned by exactly one process. A partial, corrupt,
forked, or multiply owned run is not valid target truth and must fail readiness
until it is explicitly recovered or invalidated. Admin reset, logical-time,
fault, and journal-export controls use a separate listener and credential and
are never reachable through the Redfish-shaped target surface.

## Consequences

One process may host many virtual targets, but only one process may own a
retained run. HTTP parsing and callbacks may be concurrent while one bounded
engine command stream owns mutable target state. Callback destinations are
allowlisted, redirects are disabled, and worker/queue/connection counts are
bounded.

This decision proves only the repository-owned synthetic target boundary. It
does not prove vendor, external-device, production-network, or complete
Redfish behavior.
