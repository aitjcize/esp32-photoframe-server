-- Lossy by design: the pre-connection schema can hold one connection per
-- source, so the lowest-id connection of each type is written back to the
-- settings keys and the other connections' albums and images are dropped.
-- SQLite cannot drop a column that carries a REFERENCES clause, so albums and
-- images are rebuilt without connection_id (otherwise their foreign keys
-- would point at a dropped table and every insert would fail). Dropping a
-- parent table runs its ON DELETE actions, so the child tables are saved
-- first and recreated afterwards.

INSERT OR REPLACE INTO settings (key, value)
SELECT k.key, k.value FROM (
    SELECT 'immich_url' AS key, json_extract(config, '$.url') AS value FROM connections WHERE source = 'immich' ORDER BY id LIMIT 1
) k WHERE k.value IS NOT NULL;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'immich_api_key', json_extract(config, '$.api_key') FROM connections WHERE source = 'immich' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'immich_memory_mode', json_extract(config, '$.memory_mode') FROM connections WHERE source = 'immich' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'immich_auto_sync_enabled', CASE WHEN json_extract(config, '$.auto_sync_enabled') THEN 'true' ELSE 'false' END
FROM connections WHERE source = 'immich' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'immich_auto_sync_interval_minutes', CAST(json_extract(config, '$.auto_sync_interval_minutes') AS TEXT)
FROM connections WHERE source = 'immich' ORDER BY id LIMIT 1;

INSERT OR REPLACE INTO settings (key, value)
SELECT 'synology_url', json_extract(config, '$.url') FROM connections WHERE source = 'synology_photos' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'synology_account', json_extract(config, '$.account') FROM connections WHERE source = 'synology_photos' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'synology_password', json_extract(config, '$.password') FROM connections WHERE source = 'synology_photos' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'synology_skip_cert', CASE WHEN json_extract(config, '$.skip_cert') THEN 'true' ELSE 'false' END
FROM connections WHERE source = 'synology_photos' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'synology_auto_sync_enabled', CASE WHEN json_extract(config, '$.auto_sync_enabled') THEN 'true' ELSE 'false' END
FROM connections WHERE source = 'synology_photos' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'synology_auto_sync_interval_minutes', CAST(json_extract(config, '$.auto_sync_interval_minutes') AS TEXT)
FROM connections WHERE source = 'synology_photos' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'synology_sid', json_extract(state, '$.sid') FROM connections WHERE source = 'synology_photos' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'synology_did', json_extract(state, '$.did') FROM connections WHERE source = 'synology_photos' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'synology_token', json_extract(state, '$.token') FROM connections WHERE source = 'synology_photos' ORDER BY id LIMIT 1;

-- Keep only the first connection's rows per source; the rest cannot be
-- represented and would collide on (source, external_id).
DELETE FROM albums WHERE connection_id IS NOT NULL AND connection_id NOT IN (
    SELECT MIN(id) FROM connections GROUP BY source);
DELETE FROM images WHERE connection_id IS NOT NULL AND connection_id NOT IN (
    SELECT MIN(id) FROM connections GROUP BY source);

-- Save the children of albums and images (dropping a parent would cascade
-- into them or null their references), and the AUTOINCREMENT counters of the
-- rebuilt tables: a rebuilt table restarts at its highest surviving id, which
-- would hand a deleted row's id to the next insert.
CREATE TABLE seq_backup AS SELECT name, seq FROM sqlite_sequence
    WHERE name IN ('images', 'albums', 'device_histories');
CREATE TABLE iam_backup AS SELECT image_id, album_id FROM image_album_memberships;
CREATE TABLE dam_backup AS SELECT device_id, album_id FROM device_album_mappings;
CREATE TABLE dh_backup AS SELECT id, device_id, image_id, served_at FROM device_histories;
DROP TABLE image_album_memberships;
DROP TABLE device_album_mappings;
DROP TABLE device_histories;

-- images without connection_id (the 000040 shape)
CREATE TABLE images_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_path TEXT,
    caption TEXT,
    width INTEGER,
    height INTEGER,
    orientation TEXT,
    user_id INTEGER,
    status TEXT,
    source TEXT,
    thumbnail_key TEXT,
    created_at DATETIME,
    deleted_at DATETIME,
    photo_taken_at DATETIME,
    external_id TEXT NOT NULL DEFAULT ''
);
INSERT INTO images_new (id, file_path, caption, width, height, orientation, user_id, status, source,
                        thumbnail_key, created_at, deleted_at, photo_taken_at, external_id)
    SELECT id, file_path, caption, width, height, orientation, user_id, status, source,
           thumbnail_key, created_at, deleted_at, photo_taken_at, external_id
    FROM images;
DROP TABLE images;
ALTER TABLE images_new RENAME TO images;
CREATE INDEX idx_images_deleted_at ON images(deleted_at);
CREATE INDEX idx_images_source ON images(source);
CREATE INDEX idx_images_source_created_at ON images(source, created_at);
CREATE INDEX idx_images_source_external ON images(source, external_id);
CREATE UNIQUE INDEX idx_images_source_external_unique
  ON images(source, external_id)
  WHERE external_id != '' AND deleted_at IS NULL;
CREATE INDEX idx_images_source_orientation ON images(source, orientation);

-- albums without connection_id (the 000040 shape)
CREATE TABLE albums_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT 'album',
    sync_enabled INTEGER NOT NULL DEFAULT 0,
    cover_key TEXT NOT NULL DEFAULT '',
    updated_at DATETIME,
    share_passphrase TEXT NOT NULL DEFAULT ''
);
INSERT INTO albums_new (id, source, external_id, name, kind, sync_enabled, cover_key, updated_at, share_passphrase)
    SELECT id, source, external_id, name, kind, sync_enabled, cover_key, updated_at, share_passphrase
    FROM albums;
DROP TABLE albums;
ALTER TABLE albums_new RENAME TO albums;
CREATE UNIQUE INDEX idx_albums_source_external ON albums(source, external_id);

-- The children again, as 000032 made them.
CREATE TABLE image_album_memberships (
    image_id INTEGER NOT NULL,
    album_id INTEGER NOT NULL,
    PRIMARY KEY (image_id, album_id),
    FOREIGN KEY (image_id) REFERENCES images(id) ON DELETE CASCADE,
    FOREIGN KEY (album_id) REFERENCES albums(id) ON DELETE CASCADE
);
INSERT INTO image_album_memberships (image_id, album_id)
    SELECT image_id, album_id FROM iam_backup
    WHERE image_id IN (SELECT id FROM images) AND album_id IN (SELECT id FROM albums);
CREATE INDEX idx_iam_album ON image_album_memberships(album_id);

CREATE TABLE device_album_mappings (
    device_id INTEGER NOT NULL,
    album_id INTEGER NOT NULL,
    PRIMARY KEY (device_id, album_id),
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE,
    FOREIGN KEY (album_id) REFERENCES albums(id) ON DELETE CASCADE
);
INSERT INTO device_album_mappings (device_id, album_id)
    SELECT device_id, album_id FROM dam_backup
    WHERE device_id IN (SELECT id FROM devices) AND album_id IN (SELECT id FROM albums);
CREATE INDEX idx_dam_album ON device_album_mappings(album_id);

CREATE TABLE device_histories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id INTEGER,
    image_id INTEGER,
    served_at DATETIME,
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE,
    FOREIGN KEY (image_id) REFERENCES images(id) ON DELETE SET NULL
);
INSERT INTO device_histories (id, device_id, image_id, served_at)
    SELECT id, device_id,
           CASE WHEN image_id IN (SELECT id FROM images) THEN image_id ELSE NULL END,
           served_at
    FROM dh_backup
    WHERE device_id IS NULL OR device_id IN (SELECT id FROM devices);
CREATE INDEX idx_device_histories_device_id ON device_histories(device_id);
CREATE INDEX idx_device_histories_device_served ON device_histories(device_id, served_at DESC);

DELETE FROM sqlite_sequence WHERE name IN ('images', 'albums', 'device_histories');
INSERT INTO sqlite_sequence (name, seq) SELECT name, seq FROM seq_backup;

DROP TABLE seq_backup;
DROP TABLE iam_backup;
DROP TABLE dam_backup;
DROP TABLE dh_backup;

DROP INDEX IF EXISTS idx_connections_source;
DROP TABLE connections;
