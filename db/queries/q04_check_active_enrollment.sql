-- P03-Q04: check-active-enrollment.sql
-- Access pattern: readiness ownership gate — find the active enrollment for a target.
-- Index: target_enrollments_one_active_target (partial unique on target_id WHERE state='ACTIVE')
SELECT id,
       target_id,
       campaign_id,
       campaign_target_id,
       state,
       acquired_at,
       released_at,
       release_reason
  FROM target_enrollments
 WHERE target_id = $1
   AND state = 'ACTIVE';
