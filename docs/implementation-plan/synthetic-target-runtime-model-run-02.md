# Synthetic target runtime model — Run 02

## Scope and boundary

This run extends only the in-memory deterministic simulator. It adds no
network listener, filesystem access, wall-clock ticker, database dependency,
or firmware-byte handling. The model remains owned by `core.Engine`'s single
goroutine; callers exchange immutable commands and immutable snapshots.

## State model

Each target stores immutable identity/topology fields plus current and staged
versions, reachability, health, reboot phase, request count, accepted-operation
ordinal, task list, subscriptions, event sequence, and remaining fault counts.
Operations, tasks, subscriptions, and events are values keyed by deterministic
IDs derived from scenario identity plus their creation ordinal. Snapshot slices
are sorted by ID and copied before returning so map ordering and later engine
mutation cannot leak to callers.

Lifecycle states are `accepted`, `running`, `rebooting`,
`inventory-pending`, `completed`, `healthy`, with explicit terminal exception,
missing-task, version-contradiction, and health-failed outcomes. Scheduled
events use fixed priorities: task start (10), reboot start (20), inventory
visibility (30), health evaluation (40), then emitted event (50). The existing
scheduler resolves simultaneous work by this priority, target ID, then its
insertion sequence.

## Commands and control flow

`Observe`, `AcceptUpdate`, `CreateSubscription`, `ConsumeFault`,
`ScheduleTransition`, and `AdvanceTo` are the closed command boundary. Update
acceptance increments a target request count and creates a distinct operation
and task; it does not deduplicate an identical attempt. `AdvanceTo` is the only
way lifecycle state changes. For each due scheduled transition the engine
updates target state, task state, inventory/health truth, and event sequence in
the documented order.

`ConsumeFault` validates scope and remaining count before decrementing it. A
fault is selected deterministically by scenario priority, ID, and declaration
order. Unsupported scenarios remain rejected by the scenario package.

## Affected files

- `internal/simulator/core/command.go`: closed immutable command values and
  transition/fault constants.
- `internal/simulator/core/engine.go`: private mutable model, command
  application, ID derivation, lifecycle scheduling, and defensive snapshots.
- `internal/simulator/core/*_test.go`: transition tables, ordering,
  determinism, negative invariants, cancellation, queue pressure, and
  concurrent serialization checks.
- `test/simulator/model`: a small independent lifecycle reference model used
  by tests to report directed transition and fault coverage without importing
  persistence.

## Error and limit behavior

Unknown targets, invalid lifecycle transitions, missing/oversized identifiers,
staged-version mismatches, exhausted faults, duplicate subscription IDs, and
time reversal leave state unchanged. Subscription counts obey scenario limits.
Queue admission honors context cancellation. No command returns a map or a
mutable engine-owned reference.

## Verification

Focused tests must demonstrate every healthy lifecycle edge and all supported
fault IDs using logical time only. They also verify same-trace equality,
distinct-seed identity differences, duplicate-accept visibility, monotonic task
progress, deterministic simultaneous-event ordering, state non-mutation for
rejected commands, bounded fault consumption, cancellation, queue-full
behavior, and concurrent submit serialization. Finish with full Go tests, vet,
role builds, and `git diff --check`; Linux race evidence remains a later CI
requirement.
