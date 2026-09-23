# ADR-038 — Simulator isolation and single-owner runtime

## Status

Accepted for code preparation; live capability proof is pending.

## Decision

The QEMU simulator runs only in a dedicated network with no inbound security
group rule. Operator access is Session Manager only. The simulator has an
independent EC2 role and cannot receive real-target credentials or production
routes. Instance count defaults to zero and the environment may contain at
most one simulator instance. A retained simulator run is owned by exactly one
process. A partial, corrupt, forked, or multiply owned run is not a clean BMC
and must fail readiness until it is explicitly recovered or invalidated.

## Consequences

Any future need for target allowlisting, inbound traffic, a persistent host, or
additional instances requires a new review and a measured cost record.

This decision prepares only the simulator isolation boundary. It does not
prove an AWS network, EC2 instance, QEMU image, firmware-update route, or
physical-device behavior.
