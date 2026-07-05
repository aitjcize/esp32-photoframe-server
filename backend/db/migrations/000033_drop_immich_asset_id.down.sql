-- Restore the column + index and re-derive it from external_id for Immich rows.
ALTER TABLE images ADD COLUMN immich_asset_id TEXT NOT NULL DEFAULT '';
UPDATE images SET immich_asset_id = external_id WHERE source = 'immich' AND external_id != '';
CREATE INDEX IF NOT EXISTS idx_images_source_immich ON images(source, immich_asset_id);
