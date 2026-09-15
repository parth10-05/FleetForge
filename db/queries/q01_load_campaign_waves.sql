-- P03-Q01: load-campaign-waves.sql
-- Access pattern: load all waves for a campaign in release order.
-- Index: campaign_waves_campaign_ordinal_index ON campaign_waves (campaign_id, ordinal)
SELECT id,
       campaign_id,
       ordinal,
       evaluation,
       released,
       terminal,
       created_at,
       updated_at
  FROM campaign_waves
 WHERE campaign_id = $1
 ORDER BY ordinal ASC;
