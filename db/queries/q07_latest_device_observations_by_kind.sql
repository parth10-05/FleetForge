-- P03-Q07: latest-device-observations-by-kind.sql
-- Access pattern: inspect the latest normalized evidence for a target and observation kind.
-- Index: device_observations_target_kind_time_index ON device_observations (target_id, kind, observed_at DESC)
SELECT id,
       target_id,
       target_update_id,
       kind,
       observed_at,
       received_at,
       normalized_data,
       raw_object_reference
  FROM device_observations
 WHERE target_id = $1
   AND kind = $2
 ORDER BY observed_at DESC
 LIMIT $3;
