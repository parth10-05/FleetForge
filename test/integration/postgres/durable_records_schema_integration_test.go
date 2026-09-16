//go:build postgresintegration

package postgres

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// ─── P03-05: durable records schema constraint proofs ───────────────────────

// TestDurableRecordsSchemaRejectsInvalidStepAttempts proves that step_attempts
// enforces positive attempt numbers, known outcome values, and completion
// consistency between outcome and completed_at.
func TestDurableRecordsSchemaRejectsInvalidStepAttempts(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)

	// Valid base row inserted to confirm the table is reachable.
	mustExecute(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO step_attempts (id, target_update_id, step, attempt_number, command_id, outcome) "+
			"VALUES ('00000000-0000-0000-1001-000000000001', '%s', 'PRECHECK', 1, '00000000-0000-0000-2001-000000000001', 'INTENDED')",
		fixture.targetUpdateID,
	))

	// Negative attempt number must be rejected.
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO step_attempts (id, target_update_id, step, attempt_number, command_id, outcome) "+
			"VALUES ('00000000-0000-0000-1001-000000000002', '%s', 'PRECHECK', -1, '00000000-0000-0000-2001-000000000002', 'INTENDED')",
		fixture.targetUpdateID,
	))

	// Zero attempt number must be rejected (check requires > 0).
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO step_attempts (id, target_update_id, step, attempt_number, command_id, outcome) "+
			"VALUES ('00000000-0000-0000-1001-000000000003', '%s', 'PRECHECK', 0, '00000000-0000-0000-2001-000000000003', 'INTENDED')",
		fixture.targetUpdateID,
	))

	// Unknown outcome must be rejected.
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO step_attempts (id, target_update_id, step, attempt_number, command_id, outcome) "+
			"VALUES ('00000000-0000-0000-1001-000000000004', '%s', 'PRECHECK', 2, '00000000-0000-0000-2001-000000000004', 'CORRUPT')",
		fixture.targetUpdateID,
	))

	// INTENDED outcome with a completed_at timestamp must be rejected (completion-consistency).
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO step_attempts (id, target_update_id, step, attempt_number, command_id, outcome, completed_at) "+
			"VALUES ('00000000-0000-0000-1001-000000000005', '%s', 'PRECHECK', 3, '00000000-0000-0000-2001-000000000005', 'INTENDED', now())",
		fixture.targetUpdateID,
	))

	// SUCCEEDED without completed_at must be rejected (completion-consistency).
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO step_attempts (id, target_update_id, step, attempt_number, command_id, outcome) "+
			"VALUES ('00000000-0000-0000-1001-000000000006', '%s', 'PRECHECK', 4, '00000000-0000-0000-2001-000000000006', 'SUCCEEDED')",
		fixture.targetUpdateID,
	))

	// Duplicate (target_update_id, step, attempt_number) must be rejected.
	assertSQLState(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO step_attempts (id, target_update_id, step, attempt_number, command_id, outcome) "+
			"VALUES ('00000000-0000-0000-1001-000000000007', '%s', 'PRECHECK', 1, '00000000-0000-0000-2001-000000000007', 'INTENDED')",
		fixture.targetUpdateID,
	), "23505")
}

// TestDurableRecordsSchemaRejectsInvalidDeviceObservations proves that
// device_observations enforces bounded data, non-empty kind, and time ordering.
func TestDurableRecordsSchemaRejectsInvalidDeviceObservations(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)

	// Valid base row.
	mustExecute(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO device_observations (id, target_id, kind, observed_at, normalized_data) "+
			"VALUES ('00000000-0000-0000-1002-000000000001', '%s', 'BMC_VERSION', now(), '{}')",
		fixture.targetID,
	))

	// Oversized normalized_data (> 32768 bytes) must be rejected.
	oversized := `{"x":"` + strings.Repeat("a", 32770) + `"}`
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO device_observations (id, target_id, kind, observed_at, normalized_data) "+
			"VALUES ('00000000-0000-0000-1002-000000000002', '%s', 'BMC_VERSION', now(), '%s')",
		fixture.targetID, oversized,
	))

	// Empty kind must be rejected.
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO device_observations (id, target_id, kind, observed_at, normalized_data) "+
			"VALUES ('00000000-0000-0000-1002-000000000003', '%s', '', now(), '{}')",
		fixture.targetID,
	))

	// normalized_data that is not a JSON object must be rejected.
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO device_observations (id, target_id, kind, observed_at, normalized_data) "+
			"VALUES ('00000000-0000-0000-1002-000000000004', '%s', 'BMC_VERSION', now(), '[]')",
		fixture.targetID,
	))

	// received_at more than 366 days before observed_at must be rejected.
	observedAt := time.Now().UTC().Format(time.RFC3339)
	tooEarlyReceivedAt := time.Now().Add(-370 * 24 * time.Hour).UTC().Format(time.RFC3339)
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO device_observations (id, target_id, kind, observed_at, received_at, normalized_data) "+
			"VALUES ('00000000-0000-0000-1002-000000000005', '%s', 'BMC_VERSION', '%s', '%s', '{}')",
		fixture.targetID, observedAt, tooEarlyReceivedAt,
	))
}

// TestDurableRecordsSchemaRejectsInvalidIdempotencyRecords proves that
// idempotency_records enforces hash length, route format, state vocabulary,
// completion consistency, and the unique (actor_id, route, key_hash) index.
func TestDurableRecordsSchemaRejectsInvalidIdempotencyRecords(t *testing.T) {
	_, pool, ctx := migratedHarness(t)

	// Valid base row.
	mustExecute(t, pool, ctx,
		"INSERT INTO idempotency_records (id, actor_id, route, key_hash, request_hash, command_id, owner_token, claim_expires_at) "+
			"VALUES ('00000000-0000-0000-1003-000000000001', 'actor-a', '/v1/campaigns', "+
			"'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', "+
			"'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', "+
			"'00000000-0000-0000-2003-000000000001', '00000000-0000-0000-3003-000000000001', "+
			"now() + interval '1 hour')",
	)

	// key_hash too short must be rejected.
	assertConstraint(t, pool, ctx,
		"INSERT INTO idempotency_records (id, actor_id, route, key_hash, request_hash, command_id, owner_token, claim_expires_at) "+
			"VALUES ('00000000-0000-0000-1003-000000000002', 'actor-b', '/v1/campaigns', "+
			"'short', 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', "+
			"'00000000-0000-0000-2003-000000000002', '00000000-0000-0000-3003-000000000002', "+
			"now() + interval '1 hour')",
	)

	// Route that does not start with '/' must be rejected.
	assertConstraint(t, pool, ctx,
		"INSERT INTO idempotency_records (id, actor_id, route, key_hash, request_hash, command_id, owner_token, claim_expires_at) "+
			"VALUES ('00000000-0000-0000-1003-000000000003', 'actor-c', 'v1/campaigns', "+
			"'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', "+
			"'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', "+
			"'00000000-0000-0000-2003-000000000003', '00000000-0000-0000-3003-000000000003', "+
			"now() + interval '1 hour')",
	)

	// Unknown state must be rejected.
	assertConstraint(t, pool, ctx,
		"INSERT INTO idempotency_records (id, actor_id, route, key_hash, request_hash, command_id, state, owner_token, claim_expires_at) "+
			"VALUES ('00000000-0000-0000-1003-000000000004', 'actor-d', '/v1/campaigns', "+
			"'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', "+
			"'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', "+
			"'00000000-0000-0000-2003-000000000004', 'UNKNOWN_STATE', "+
			"'00000000-0000-0000-3003-000000000004', now() + interval '1 hour')",
	)

	// COMPLETED state without completed_at must be rejected.
	assertConstraint(t, pool, ctx,
		"INSERT INTO idempotency_records (id, actor_id, route, key_hash, request_hash, command_id, state, owner_token, claim_expires_at) "+
			"VALUES ('00000000-0000-0000-1003-000000000005', 'actor-e', '/v1/campaigns', "+
			"'dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd', "+
			"'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', "+
			"'00000000-0000-0000-2003-000000000005', 'COMPLETED', "+
			"'00000000-0000-0000-3003-000000000005', now() + interval '1 hour')",
	)

	// Duplicate (actor_id, route, key_hash) must be rejected.
	assertSQLState(t, pool, ctx,
		"INSERT INTO idempotency_records (id, actor_id, route, key_hash, request_hash, command_id, owner_token, claim_expires_at) "+
			"VALUES ('00000000-0000-0000-1003-000000000006', 'actor-a', '/v1/campaigns', "+
			"'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', "+
			"'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', "+
			"'00000000-0000-0000-2003-000000000006', '00000000-0000-0000-3003-000000000006', "+
			"now() + interval '1 hour')",
		"23505",
	)
}

// TestDurableRecordsSchemaRejectsInvalidDecisionInputs proves that
// decision_inputs enforces known aggregate types, non-negative revisions,
// non-empty version fields, bounded JSON, and the unique composite key.
func TestDurableRecordsSchemaRejectsInvalidDecisionInputs(t *testing.T) {
	_, pool, ctx := migratedHarness(t)

	// Valid base row.
	mustExecute(t, pool, ctx,
		"INSERT INTO decision_inputs (id, aggregate_type, aggregate_id, aggregate_revision, decision_sequence, "+
			"logical_time, reducer_version, policy_version, normalized_facts, result, reason_code) "+
			"VALUES ('00000000-0000-0000-1004-000000000001', 'CAMPAIGN', '00000000-0000-0000-4004-000000000001', "+
			"0, 0, now(), 'v1', 'policy-v1', '{}', 'ALLOWED', 'campaign.validation.accepted')",
	)

	// Unknown aggregate_type must be rejected.
	assertConstraint(t, pool, ctx,
		"INSERT INTO decision_inputs (id, aggregate_type, aggregate_id, aggregate_revision, decision_sequence, "+
			"logical_time, reducer_version, policy_version, normalized_facts, result, reason_code) "+
			"VALUES ('00000000-0000-0000-1004-000000000002', 'UNKNOWN_TYPE', '00000000-0000-0000-4004-000000000002', "+
			"0, 0, now(), 'v1', 'policy-v1', '{}', 'ALLOWED', 'campaign.validation.accepted')",
	)

	// Negative aggregate_revision must be rejected.
	assertConstraint(t, pool, ctx,
		"INSERT INTO decision_inputs (id, aggregate_type, aggregate_id, aggregate_revision, decision_sequence, "+
			"logical_time, reducer_version, policy_version, normalized_facts, result, reason_code) "+
			"VALUES ('00000000-0000-0000-1004-000000000003', 'CAMPAIGN', '00000000-0000-0000-4004-000000000003', "+
			"-1, 0, now(), 'v1', 'policy-v1', '{}', 'ALLOWED', 'campaign.validation.accepted')",
	)

	// Empty reducer_version must be rejected.
	assertConstraint(t, pool, ctx,
		"INSERT INTO decision_inputs (id, aggregate_type, aggregate_id, aggregate_revision, decision_sequence, "+
			"logical_time, reducer_version, policy_version, normalized_facts, result, reason_code) "+
			"VALUES ('00000000-0000-0000-1004-000000000004', 'CAMPAIGN', '00000000-0000-0000-4004-000000000004', "+
			"1, 0, now(), '', 'policy-v1', '{}', 'ALLOWED', 'campaign.validation.accepted')",
	)

	// Duplicate (aggregate_type, aggregate_id, aggregate_revision, decision_sequence) must be rejected.
	assertSQLState(t, pool, ctx,
		"INSERT INTO decision_inputs (id, aggregate_type, aggregate_id, aggregate_revision, decision_sequence, "+
			"logical_time, reducer_version, policy_version, normalized_facts, result, reason_code) "+
			"VALUES ('00000000-0000-0000-1004-000000000005', 'CAMPAIGN', '00000000-0000-0000-4004-000000000001', "+
			"0, 0, now(), 'v1', 'policy-v1', '{}', 'ALLOWED', 'campaign.validation.accepted')",
		"23505",
	)
}

// TestDurableRecordsSchemaAuditEventsAreAppendOnly proves that the
// reject_audit_event_mutation trigger blocks UPDATE and DELETE on audit_events.
func TestDurableRecordsSchemaAuditEventsAreAppendOnly(t *testing.T) {
	_, pool, ctx := migratedHarness(t)

	mustExecute(t, pool, ctx,
		"INSERT INTO audit_events (aggregate_type, aggregate_id, aggregate_revision, event_sequence, actor_id, action, reason_code) "+
			"VALUES ('CAMPAIGN', '00000000-0000-0000-5005-000000000001', 0, 0, 'system', 'CREATED', 'campaign.validation.accepted')",
	)

	// UPDATE must be rejected by the trigger (SQLSTATE 55000 — object not in prerequisite state).
	assertSQLState(t, pool, ctx,
		"UPDATE audit_events SET actor_id = 'tampered' WHERE aggregate_type = 'CAMPAIGN'",
		"55000",
	)

	// DELETE must also be rejected by the trigger.
	assertSQLState(t, pool, ctx,
		"DELETE FROM audit_events WHERE aggregate_type = 'CAMPAIGN'",
		"55000",
	)

	// Unknown aggregate_type must be rejected by check constraint.
	assertConstraint(t, pool, ctx,
		"INSERT INTO audit_events (aggregate_type, aggregate_id, aggregate_revision, event_sequence, actor_id, action, reason_code) "+
			"VALUES ('UNKNOWN', '00000000-0000-0000-5005-000000000002', 0, 0, 'system', 'CREATED', 'campaign.validation.accepted')",
	)

	// Negative aggregate_revision must be rejected.
	assertConstraint(t, pool, ctx,
		"INSERT INTO audit_events (aggregate_type, aggregate_id, aggregate_revision, event_sequence, actor_id, action, reason_code) "+
			"VALUES ('CAMPAIGN', '00000000-0000-0000-5005-000000000003', -1, 0, 'system', 'CREATED', 'campaign.validation.accepted')",
	)

	// Duplicate (aggregate_type, aggregate_id, aggregate_revision, event_sequence) must be rejected.
	assertSQLState(t, pool, ctx,
		"INSERT INTO audit_events (aggregate_type, aggregate_id, aggregate_revision, event_sequence, actor_id, action, reason_code) "+
			"VALUES ('CAMPAIGN', '00000000-0000-0000-5005-000000000001', 0, 0, 'system', 'CREATED', 'campaign.validation.accepted')",
		"23505",
	)
}

// TestDurableRecordsSchemaRejectsInvalidOutboxEvents proves that outbox_events
// enforces known aggregate types, non-negative revisions, non-empty event types,
// positive schema versions, bounded JSON payloads, and publication consistency.
func TestDurableRecordsSchemaRejectsInvalidOutboxEvents(t *testing.T) {
	_, pool, ctx := migratedHarness(t)

	// Valid base row.
	mustExecute(t, pool, ctx,
		"INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, aggregate_revision, event_type, schema_version, payload) "+
			"VALUES ('00000000-0000-0000-1006-000000000001', 'CAMPAIGN', '00000000-0000-0000-6006-000000000001', 0, 'campaign.created', 1, '{}')",
	)

	// Unknown aggregate_type must be rejected.
	assertConstraint(t, pool, ctx,
		"INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, aggregate_revision, event_type, schema_version, payload) "+
			"VALUES ('00000000-0000-0000-1006-000000000002', 'UNKNOWN', '00000000-0000-0000-6006-000000000002', 0, 'campaign.created', 1, '{}')",
	)

	// Negative aggregate_revision must be rejected.
	assertConstraint(t, pool, ctx,
		"INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, aggregate_revision, event_type, schema_version, payload) "+
			"VALUES ('00000000-0000-0000-1006-000000000003', 'CAMPAIGN', '00000000-0000-0000-6006-000000000003', -1, 'campaign.created', 1, '{}')",
	)

	// Zero schema_version must be rejected.
	assertConstraint(t, pool, ctx,
		"INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, aggregate_revision, event_type, schema_version, payload) "+
			"VALUES ('00000000-0000-0000-1006-000000000004', 'CAMPAIGN', '00000000-0000-0000-6006-000000000004', 0, 'campaign.created', 0, '{}')",
	)

	// payload that is not a JSON object must be rejected.
	assertConstraint(t, pool, ctx,
		"INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, aggregate_revision, event_type, schema_version, payload) "+
			"VALUES ('00000000-0000-0000-1006-000000000005', 'CAMPAIGN', '00000000-0000-0000-6006-000000000005', 0, 'campaign.created', 1, '\"string\"')",
	)

	// PUBLISHED state without published_at must be rejected (publication consistency).
	assertConstraint(t, pool, ctx,
		"INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, aggregate_revision, event_type, schema_version, payload, delivery_state) "+
			"VALUES ('00000000-0000-0000-1006-000000000006', 'CAMPAIGN', '00000000-0000-0000-6006-000000000006', 0, 'campaign.created', 1, '{}', 'PUBLISHED')",
	)

	// Unknown delivery_state must be rejected.
	assertConstraint(t, pool, ctx,
		"INSERT INTO outbox_events (event_id, aggregate_type, aggregate_id, aggregate_revision, event_type, schema_version, payload, delivery_state) "+
			"VALUES ('00000000-0000-0000-1006-000000000007', 'CAMPAIGN', '00000000-0000-0000-6006-000000000007', 0, 'campaign.created', 1, '{}', 'CORRUPT')",
	)
}

// TestDurableRecordsSchemaRejectsInvalidDueWork proves that due_work enforces
// known subject types, non-negative revisions, positive schema versions, known
// delivery states, lease consistency, and the semantic unique key.
func TestDurableRecordsSchemaRejectsInvalidDueWork(t *testing.T) {
	_, pool, ctx := migratedHarness(t)
	fixture := seedOwnershipFixture(t, pool, ctx)

	// Valid base row.
	mustExecute(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO due_work (id, command_id, subject_type, subject_id, work_type, schema_version, not_before, expected_revision) "+
			"VALUES ('00000000-0000-0000-1007-000000000001', '00000000-0000-0000-2007-000000000001', "+
			"'TARGET_UPDATE', '%s', 'PRECHECK', 1, now(), 0)",
		fixture.targetUpdateID,
	))

	// Unknown subject_type must be rejected.
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO due_work (id, command_id, subject_type, subject_id, work_type, schema_version, not_before, expected_revision) "+
			"VALUES ('00000000-0000-0000-1007-000000000002', '00000000-0000-0000-2007-000000000002', "+
			"'UNKNOWN', '%s', 'PRECHECK', 1, now(), 0)",
		fixture.targetUpdateID,
	))

	// Negative expected_revision must be rejected.
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO due_work (id, command_id, subject_type, subject_id, work_type, schema_version, not_before, expected_revision) "+
			"VALUES ('00000000-0000-0000-1007-000000000003', '00000000-0000-0000-2007-000000000003', "+
			"'TARGET_UPDATE', '%s', 'PRECHECK', 1, now(), -1)",
		fixture.targetUpdateID,
	))

	// Zero schema_version must be rejected.
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO due_work (id, command_id, subject_type, subject_id, work_type, schema_version, not_before, expected_revision) "+
			"VALUES ('00000000-0000-0000-1007-000000000004', '00000000-0000-0000-2007-000000000004', "+
			"'TARGET_UPDATE', '%s', 'PRECHECK', 0, now(), 0)",
		fixture.targetUpdateID,
	))

	// CLAIMED delivery_state without lease_token/owner/expiry must be rejected.
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO due_work (id, command_id, subject_type, subject_id, work_type, schema_version, not_before, expected_revision, delivery_state) "+
			"VALUES ('00000000-0000-0000-1007-000000000005', '00000000-0000-0000-2007-000000000005', "+
			"'TARGET_UPDATE', '%s', 'PRECHECK', 1, now(), 0, 'CLAIMED')",
		fixture.targetUpdateID,
	))

	// Semantic unique key (subject_type, subject_id, work_type, command_id) must be rejected on duplicate.
	assertSQLState(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO due_work (id, command_id, subject_type, subject_id, work_type, schema_version, not_before, expected_revision) "+
			"VALUES ('00000000-0000-0000-1007-000000000006', '00000000-0000-0000-2007-000000000001', "+
			"'TARGET_UPDATE', '%s', 'PRECHECK', 1, now(), 0)",
		fixture.targetUpdateID,
	), "23505")

	// Unknown delivery_state must be rejected.
	assertConstraint(t, pool, ctx, fmt.Sprintf(
		"INSERT INTO due_work (id, command_id, subject_type, subject_id, work_type, schema_version, not_before, expected_revision, delivery_state) "+
			"VALUES ('00000000-0000-0000-1007-000000000007', '00000000-0000-0000-2007-000000000007', "+
			"'TARGET_UPDATE', '%s', 'PRECHECK', 1, now(), 0, 'BOGUS')",
		fixture.targetUpdateID,
	))
}
