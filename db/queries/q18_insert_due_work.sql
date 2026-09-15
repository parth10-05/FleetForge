-- P03-Q18: insert-due-work.sql
-- Access pattern: schedule a due-work intent within a unit-of-work transaction.
-- Semantic unique key prevents duplicate work entries for the same command.
-- name: InsertDueWork :exec
INSERT INTO due_work
    (id, command_id, subject_type, subject_id, work_type, schema_version,
     not_before, expected_revision, expected_command_epoch)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
