-- preflight-candidate-drift.sql
-- Access pattern: compare immutable candidate target facts to current
-- inventory. It is diagnostic-only and never changes frozen membership.
-- name: PreflightCandidateDrift :many
SELECT frozen.target_id,
       frozen.frozen_target_revision,
       frozen.site_key AS frozen_site_key,
       frozen.rack_key AS frozen_rack_key,
       frozen.target_family AS frozen_family,
       live.revision AS current_revision,
       live.administrative_state AS current_administrative_state,
       live.site_key AS current_site_key,
       live.rack_key AS current_rack_key,
       family.value AS current_family
  FROM preflight_candidate_targets frozen
  LEFT JOIN targets live ON live.id = frozen.target_id
  LEFT JOIN target_labels family ON family.target_id = frozen.target_id AND family.key = 'family'
 WHERE frozen.candidate_id = $1
 ORDER BY frozen.target_id ASC;
