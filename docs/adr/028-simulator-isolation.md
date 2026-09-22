# ADR 028: Simulator isolation

## Status

Accepted for code preparation; live capability proof is pending.

## Decision

The QEMU simulator runs only in a dedicated network with no inbound security
group rule. Operator access is Session Manager only. The simulator has an
independent EC2 role and cannot receive real-target credentials or production
routes. Instance count defaults to zero and the environment may contain at
most one simulator instance.

## Consequences

Any future need for target allowlisting, inbound traffic, a persistent host, or
additional instances requires a new review and a measured cost record.
