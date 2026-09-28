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
)
SELECT t.id, t.revision, t.site_key, t.rack_key, family.value AS family
  FROM targets t
  JOIN target_labels family
    ON family.target_id = t.id AND family.key = 'family'
 WHERE t.administrative_state = 'ACTIVE'
   AND t.trust_metadata_complete
   AND NOT EXISTS (
       SELECT 1
         FROM requested requirement
        WHERE NOT EXISTS (
            SELECT 1
              FROM target_labels actual
             WHERE actual.target_id = t.id
               AND actual.key = requirement.key
               AND actual.value = requirement.value
        )
   )
 ORDER BY t.id ASC
 LIMIT $3;
