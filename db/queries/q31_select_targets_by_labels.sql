-- P02-Q31: select-targets-by-labels.sql
-- Access pattern: evaluate the bounded exact-label selector using only typed
-- array parameters. The query has no caller-controlled SQL fragments and its
-- UUID ordering is the canonical membership order before snapshot freezing.
-- name: SelectTargetsByLabels :many
WITH requested(key, value) AS (
    SELECT keys.key, values.value
      FROM unnest($1::varchar[]) WITH ORDINALITY AS keys(key, ordinal)
      JOIN unnest($2::varchar[]) WITH ORDINALITY AS values(value, ordinal)
        ON values.ordinal = keys.ordinal
), matching_targets AS (
    SELECT actual.target_id
      FROM requested requirement
      JOIN target_labels actual
        ON actual.key = requirement.key
       AND actual.value = requirement.value
     GROUP BY actual.target_id
    HAVING count(*) = (SELECT count(*) FROM requested)
)
SELECT t.id, t.revision, t.site_key, t.rack_key, family.value AS family
  FROM matching_targets matching
  JOIN targets t ON t.id = matching.target_id
  JOIN target_labels family
    ON family.target_id = t.id AND family.key = 'family'
 WHERE t.administrative_state = 'ACTIVE'
   AND t.trust_metadata_complete
 ORDER BY t.id ASC
 LIMIT $3;
