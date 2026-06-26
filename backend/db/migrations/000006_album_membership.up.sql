-- Persist photo-source albums (Immich/Synology) as first-class rows, with a
-- per-album sync flag. Immich "modes" (all/favorites/memories) are modeled as
-- virtual albums (kind='virtual'). external_id holds the album UUID/id or one
-- of '__all__' / '__favorites__' / '__memories__'.
CREATE TABLE IF NOT EXISTS albums (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT 'album',
    sync_enabled INTEGER NOT NULL DEFAULT 0,
    asset_count INTEGER NOT NULL DEFAULT 0,
    cover_key TEXT NOT NULL DEFAULT '',
    updated_at DATETIME
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_albums_source_external ON albums(source, external_id);

-- Many-to-many: an image (asset) can belong to several albums. Lets us keep
-- one images row per asset while still filtering a device's pool by album.
CREATE TABLE IF NOT EXISTS image_album_memberships (
    image_id INTEGER NOT NULL,
    album_id INTEGER NOT NULL,
    PRIMARY KEY (image_id, album_id)
);
CREATE INDEX IF NOT EXISTS idx_iam_album ON image_album_memberships(album_id);

-- Which albums each device draws from (multi-album per device).
CREATE TABLE IF NOT EXISTS device_album_mappings (
    device_id INTEGER NOT NULL,
    album_id INTEGER NOT NULL,
    PRIMARY KEY (device_id, album_id)
);
