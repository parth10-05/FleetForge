# Synthetic target durable state — Run 03

## Scope

This run replaces the preparation journal append path and retained-run
ownership/snapshot assumptions. It does not add HTTP, callback dispatch, cloud,
or database behavior.

## Journal contract

Journal records use a closed v2 kind set and one typed, safe payload object.
The payload is encoded canonically into the hash chain and rejects unknown JSON
fields, control characters, oversized values, credential-shaped keys, raw URLs,
and artifact bytes. Receipt and acceptance records are always synchronously
durable. Diagnostics may be buffered only through a bounded explicit policy.

`journal.Writer` validates the complete existing chain once during `OpenWriter`,
retains sequence/previous-hash state in memory, serializes append requests with
a mutex, writes exactly one line, syncs when required, and then advances its
in-memory state. The old `Append` remains only as a compatibility wrapper for
test preparation and opens/closes a writer; production callers use `Writer`.

## Snapshot and recovery

Snapshots contain schema version, logical time, target/task/fault/subscription
state, journal checkpoint sequence/hash, and a deterministic state digest.
Write uses a same-directory temporary file, file sync, rename, and best-effort
directory sync where the platform permits. Recovery validates manifest and
scenario digest, full journal chain, snapshot checkpoint/hash, and snapshot
digest before exposing a run; any mismatch writes a bounded invalidation marker
and fails closed without truncation.

## Ownership

An OS advisory file lock owns a live run. Platform-specific implementations use
`flock` on Unix and `LockFileEx` on Windows. A small metadata file is
diagnostic-only and cannot establish ownership. The lock handle remains open
until release; OS cleanup after process exit makes stale-sentinel recovery
possible.

## Verification

Tests cover unknown/unsafe records, synchronous receipt/acceptance writes,
writer serialization and a 10,000-record linear-work guard, tampered and
partial journals, snapshot checkpoint/digest invalidation, live second-owner
rejection, release/reacquisition, and accepted-record replay cardinality.
Linux race and cross-process crash evidence remain CI requirements.
