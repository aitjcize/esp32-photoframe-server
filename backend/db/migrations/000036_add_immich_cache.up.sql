CREATE TABLE immich_caches (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    image_id        INTEGER NOT NULL REFERENCES images(id) ON DELETE CASCADE,
    asset_id        TEXT NOT NULL UNIQUE,
    file_path       TEXT NOT NULL,
    width           INTEGER NOT NULL,
    height          INTEGER NOT NULL,
    size_bytes      INTEGER NOT NULL DEFAULT 0,
    cached_at       DATETIME NOT NULL
);
CREATE INDEX idx_immich_caches_image_id ON immich_caches(image_id);
CREATE INDEX idx_immich_caches_cached_at ON immich_caches(cached_at);
