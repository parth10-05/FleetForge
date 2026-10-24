# V1-P05 callback ordering proof stabilization plan

## Objective

Make the composed duplicate and late callback cases prove their intended
post-acceptance behavior without weakening the production rule that callbacks
received before a durable accepted attempt are merely `UNMATCHED` advisory
receipts and never create a target event head or work item.

## Root cause

`RecordEventHint` creates `execution_event_heads` only when a callback matches
an attempt with `status='ACCEPTED'` and a target update in
`state='UPDATE_ACCEPTED'`. The composed callback tests currently use 100 ms
synthetic lifecycle timing. On a real remote PostgreSQL harness, the simulator
can dispatch its callback before the test's independent
`CommitSubmitResult` transaction has committed those states. The handler then
correctly stores a receipt with `UNMATCHED` disposition; the test incorrectly
expects an event head from that early receipt.

## Design

1. Preserve `RecordEventHint` matching semantics and its no-authority rule.
   Do not retry or rewrite an `UNMATCHED` callback into a scheduled callback.
2. In only the composed callback fixtures, use a sufficiently delayed event
   schedule after task/acceptance so the test commits the durable accepted
   state before the first callback can fire. Keep the test's overall context
   bounded.
3. Retain duplicate/late fault behavior after that delayed baseline event:
   duplicate still yields one scheduled receipt plus one duplicate receipt;
   late still yields one scheduled receipt plus one late receipt and advances
   the head only to the newer sequence.
4. Add a narrow unit/integration assertion that an explicitly early callback
   remains `UNMATCHED` and leaves no event head, guarding against accidental
   callback-authority expansion.

## Affected surfaces

- `test/fault/composed_harness_integration_test.go`: callback-only timing
  setup for RF-F01/RF-F02 and the existing durable-head assertions.
- `internal/persistence/postgres/event_hints_integration_test.go` (or the
  established event-hint test surface): early unmatched callback negative
  control if it is not already proven.

## Verification

1. Run the duplicate and late composed cases independently against direct
   isolated PostgreSQL.
2. Run the full `test/fault` package against the same endpoint.
3. Confirm the hosted exact-SHA fault-harness step passes and emits evidence.
4. Confirm no production callback code, target transition, admission,
   reservation, or submission authority changes.
