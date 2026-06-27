-- Hot path: PickRandomDBPhoto filters
--   WHERE source = ? AND orientation IN (...)
-- on every device fetch. A source-leading composite with orientation lets
-- SQLite satisfy both predicates from the index.
CREATE INDEX IF NOT EXISTS idx_images_source_orientation ON images(source, orientation);

-- Gallery pagination lists a source's photos ordered by created_at. The
-- source-leading composite serves the WHERE source = ? ... ORDER BY created_at
-- scan without a filesort.
CREATE INDEX IF NOT EXISTS idx_images_source_created_at ON images(source, created_at);

-- Reverse lookups / deletes by album: removing an album must purge its
-- device_album_mappings rows. Only a (device_id, album_id) PRIMARY KEY exists,
-- which cannot serve an album_id-only predicate.
CREATE INDEX IF NOT EXISTS idx_dam_album ON device_album_mappings(album_id);
