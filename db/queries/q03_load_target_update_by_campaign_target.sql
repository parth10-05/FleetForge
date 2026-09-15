-- P03-Q03: load-target-update-by-campaign-target.sql
-- Access pattern: load a target update by frozen campaign target identity.
-- Index: target_updates_campaign_target_index ON target_updates (campaign_target_id)
SELECT id,
       campaign_target_id,
       state,
       revision,
       command_epoch,
       resume_step,
       expected_firmware_version,
       created_at,
       updated_at
  FROM target_updates
 WHERE campaign_target_id = $1;
