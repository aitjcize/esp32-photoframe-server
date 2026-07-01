-- Enforce image dedup at the DB level for the album-sync sources (immich,
-- synology, unsplash, pexels), which key each asset by a stable per-source
-- external_id. This is belt-and-suspenders behind the app-level upsert in
-- upsertAlbumAssets: with clear-before-resync removed, a failed sync no longer
-- wipes the gallery, so the only remaining dedup guarantee is external_id — make
-- it a hard constraint.
--
-- Partial index, for two reasons:
--   * Sources that don't populate external_id (gallery uploads, google_photos,
--     AI generation) leave it '' and legitimately repeat, so exclude ''.
--   * deleted_at IS NULL scopes uniqueness to live rows, matching GORM's default
--     soft-delete query scope — a soft-deleted row must not block re-inserting
--     the same asset.

-- Collapse any pre-existing duplicates first, or CREATE UNIQUE INDEX fails.
-- Keep the lowest id per (source, external_id); drop the rest and their album
-- memberships. The next sync re-creates the survivor's memberships, so no album
-- permanently loses the photo.
DELETE FROM image_album_memberships WHERE image_id IN (
  SELECT id FROM images
  WHERE external_id != '' AND deleted_at IS NULL
    AND id NOT IN (
      SELECT MIN(id) FROM images
      WHERE external_id != '' AND deleted_at IS NULL
      GROUP BY source, external_id
    )
);

DELETE FROM images
WHERE external_id != '' AND deleted_at IS NULL
  AND id NOT IN (
    SELECT MIN(id) FROM images
    WHERE external_id != '' AND deleted_at IS NULL
    GROUP BY source, external_id
  );

CREATE UNIQUE INDEX IF NOT EXISTS idx_images_source_external_unique
  ON images(source, external_id)
  WHERE external_id != '' AND deleted_at IS NULL;
