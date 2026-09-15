-- P03-Q08: target-update-evidence-timeline.sql
-- Access pattern: reconstruct a target-update evidence timeline.
-- Index: device_observations_update_time_index ON device_observations (target_update_id, observed_at DESC)
--        WHERE target_update_id IS NOT NULL
SELECT id,
       target_id,
       target_update_id,
       kind,
       observed_at,
       received_at,
       normalized_data,
       raw_object_reference
  FROM device_observations
 WHERE target_update_id = $1
 ORDER BY observed_at DESC;
