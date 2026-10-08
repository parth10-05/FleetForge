-- +goose Up
-- P05: a persisted retry must identify one exact execution boundary. Generic
-- SUBMITTING/WAITING_REBOOT/VERIFYING values could let an observer re-enter
-- firmware submission and are therefore migrated before P05 work is routed.
UPDATE target_updates
   SET resume_step = CASE resume_step
       WHEN 'SUBMITTING' THEN 'SUBMIT_ADMISSION'
       WHEN 'WAITING_REBOOT' THEN 'REBOOT_WAIT'
       WHEN 'VERIFYING' THEN 'VERIFY_VERSION'
       ELSE resume_step
   END
 WHERE resume_step IN ('SUBMITTING', 'WAITING_REBOOT', 'VERIFYING');

ALTER TABLE target_updates
    DROP CONSTRAINT target_updates_resume_step_valid,
    ADD CONSTRAINT target_updates_resume_step_valid
        CHECK (resume_step IS NULL OR resume_step IN ('PRECHECK', 'SUBMIT_ADMISSION', 'TASK_OBSERVE', 'REBOOT_WAIT', 'VERIFY_VERSION', 'VERIFY_HEALTH', 'AMBIGUITY_RECONCILE'));

-- +goose Down
SELECT 1;
