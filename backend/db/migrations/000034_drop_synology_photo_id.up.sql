-- Drop the legacy per-source synology_photo_id column; Synology assets now use
-- the generic external_id (backfilled from synology_photo_id in migration 000029,
-- and all readers were switched over). Drop its index first.
DROP INDEX IF EXISTS idx_images_source_synology;
ALTER TABLE images DROP COLUMN synology_photo_id;
