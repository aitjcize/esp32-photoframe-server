-- Drop the legacy per-source immich_asset_id column; Immich assets now use the
-- generic external_id (backfilled from immich_asset_id in migration 000029, and
-- the sole reader was switched over). Drop its index first.
DROP INDEX IF EXISTS idx_images_source_immich;
ALTER TABLE images DROP COLUMN immich_asset_id;
