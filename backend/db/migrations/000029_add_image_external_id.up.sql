-- Generic per-source external id for image dedup, so album-sync sources don't
-- each need a bespoke id column like immich_asset_id / synology_photo_id. New
-- sources (unsplash/pexels/artic/...) use this; immich/synology are backfilled
-- onto it here and switch to writing it in the same release.
ALTER TABLE images ADD COLUMN external_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_images_source_external ON images(source, external_id);

UPDATE images SET external_id = immich_asset_id
  WHERE source = 'immich' AND immich_asset_id != '';
UPDATE images SET external_id = CAST(synology_photo_id AS TEXT)
  WHERE source = 'synology_photos' AND synology_photo_id != 0;
