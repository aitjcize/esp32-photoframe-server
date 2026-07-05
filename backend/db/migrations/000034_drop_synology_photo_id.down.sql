-- Restore the column + index and re-derive it from external_id for Synology rows.
ALTER TABLE images ADD COLUMN synology_photo_id INTEGER NOT NULL DEFAULT 0;
UPDATE images SET synology_photo_id = CAST(external_id AS INTEGER) WHERE source = 'synology_photos' AND external_id != '';
CREATE INDEX IF NOT EXISTS idx_images_source_synology ON images(source, synology_photo_id);
