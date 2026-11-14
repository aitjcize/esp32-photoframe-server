-- Multiple connections per provider (several Immich servers, several Synology
-- accounts). Until now the source string ("immich", "synology_photos") was at
-- once the provider type, the connection identity and the namespace of every
-- album and photo, and each provider's credentials were flat global keys in
-- the settings table. A connection row now holds one configured instance;
-- albums and images point at the connection they came from, and an external
-- id is unique per connection instead of per source type.
--
-- Config (what the user entered, credentials included) and state (what the
-- server learned, e.g. a DSM session) are JSON columns, kept apart so a user
-- edit never races a session refresh. The Google OAuth app client id and
-- secret stay global settings: they belong to the app, not to an account.

CREATE TABLE connections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    config TEXT NOT NULL DEFAULT '{}',
    state TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME,
    updated_at DATETIME
);
CREATE INDEX idx_connections_source ON connections(source);

-- One Immich connection out of the old global keys. Created when credentials
-- exist, or when any Immich album or image row exists (a user who cleared the
-- URL but kept albums still needs a parent row; it starts disabled).
INSERT INTO connections (source, name, enabled, config, state, created_at, updated_at)
SELECT 'immich', 'Immich',
    CASE WHEN COALESCE((SELECT value FROM settings WHERE key = 'immich_url'), '') != ''
          AND COALESCE((SELECT value FROM settings WHERE key = 'immich_api_key'), '') != ''
         THEN 1 ELSE 0 END,
    json_object(
        'url', COALESCE((SELECT value FROM settings WHERE key = 'immich_url'), ''),
        'api_key', COALESCE((SELECT value FROM settings WHERE key = 'immich_api_key'), ''),
        'memory_mode', CASE WHEN COALESCE((SELECT value FROM settings WHERE key = 'immich_memory_mode'), '') = 'latest'
                            THEN 'latest' ELSE 'all' END,
        'auto_sync_enabled', json(CASE WHEN lower(COALESCE((SELECT value FROM settings WHERE key = 'immich_auto_sync_enabled'), '')) = 'true'
                                       THEN 'true' ELSE 'false' END),
        'auto_sync_interval_minutes', CAST(COALESCE(NULLIF((SELECT value FROM settings WHERE key = 'immich_auto_sync_interval_minutes'), ''), '60') AS INTEGER)
    ),
    '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE COALESCE((SELECT value FROM settings WHERE key = 'immich_url'), '') != ''
   OR EXISTS (SELECT 1 FROM albums WHERE source = 'immich')
   OR EXISTS (SELECT 1 FROM images WHERE source = 'immich');

-- Same for Synology Photos. The DSM session (sid/did/token) goes to state.
INSERT INTO connections (source, name, enabled, config, state, created_at, updated_at)
SELECT 'synology_photos', 'Synology Photos',
    CASE WHEN COALESCE((SELECT value FROM settings WHERE key = 'synology_url'), '') != ''
          AND COALESCE((SELECT value FROM settings WHERE key = 'synology_account'), '') != ''
          AND COALESCE((SELECT value FROM settings WHERE key = 'synology_password'), '') != ''
         THEN 1 ELSE 0 END,
    json_object(
        'url', COALESCE((SELECT value FROM settings WHERE key = 'synology_url'), ''),
        'account', COALESCE((SELECT value FROM settings WHERE key = 'synology_account'), ''),
        'password', COALESCE((SELECT value FROM settings WHERE key = 'synology_password'), ''),
        'skip_cert', json(CASE WHEN lower(COALESCE((SELECT value FROM settings WHERE key = 'synology_skip_cert'), '')) = 'true'
                               THEN 'true' ELSE 'false' END),
        'auto_sync_enabled', json(CASE WHEN lower(COALESCE((SELECT value FROM settings WHERE key = 'synology_auto_sync_enabled'), '')) = 'true'
                                       THEN 'true' ELSE 'false' END),
        'auto_sync_interval_minutes', CAST(COALESCE(NULLIF((SELECT value FROM settings WHERE key = 'synology_auto_sync_interval_minutes'), ''), '60') AS INTEGER)
    ),
    json_object(
        'sid', COALESCE((SELECT value FROM settings WHERE key = 'synology_sid'), ''),
        'did', COALESCE((SELECT value FROM settings WHERE key = 'synology_did'), ''),
        'token', COALESCE((SELECT value FROM settings WHERE key = 'synology_token'), '')
    ),
    CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE COALESCE((SELECT value FROM settings WHERE key = 'synology_url'), '') != ''
   OR EXISTS (SELECT 1 FROM albums WHERE source = 'synology_photos')
   OR EXISTS (SELECT 1 FROM images WHERE source = 'synology_photos');

-- Albums and images get their parent. Nullable: topic sources (unsplash,
-- pexels), gallery uploads and Google Photos have no connection.
ALTER TABLE albums ADD COLUMN connection_id INTEGER REFERENCES connections(id) ON DELETE CASCADE;
ALTER TABLE images ADD COLUMN connection_id INTEGER REFERENCES connections(id) ON DELETE CASCADE;

UPDATE albums SET connection_id = (SELECT id FROM connections WHERE connections.source = albums.source)
WHERE source IN ('immich', 'synology_photos');
UPDATE images SET connection_id = (SELECT id FROM connections WHERE connections.source = images.source)
WHERE source IN ('immich', 'synology_photos');

-- The legacy single-album settings were materialized lazily at sync time
-- (ensureGlobalAlbumSeed) for installs that predate the album picker. Do it
-- here instead, once, so that code can go: an Immich mode or album id becomes
-- a sync-enabled album row when the connection has no albums yet.
INSERT INTO albums (source, external_id, name, kind, sync_enabled, cover_key, updated_at, connection_id)
SELECT 'immich',
    CASE m.mode WHEN 'all' THEN '__all__' WHEN 'favorites' THEN '__favorites__'
                WHEN 'memories' THEN '__memories__' ELSE m.album_id END,
    CASE m.mode WHEN 'all' THEN 'All Photos' WHEN 'favorites' THEN 'Favorites'
                WHEN 'memories' THEN 'Memories' ELSE m.album_id END,
    CASE WHEN m.mode IN ('all', 'favorites', 'memories') THEN 'virtual' ELSE 'album' END,
    1, '', CURRENT_TIMESTAMP, c.id
FROM connections c,
     (SELECT COALESCE((SELECT value FROM settings WHERE key = 'immich_source_mode'), '') AS mode,
             COALESCE((SELECT value FROM settings WHERE key = 'immich_album_id'), '') AS album_id) m
WHERE c.source = 'immich'
  AND NOT EXISTS (SELECT 1 FROM albums WHERE source = 'immich')
  AND (m.mode IN ('all', 'favorites', 'memories') OR m.album_id != '');

INSERT INTO albums (source, external_id, name, kind, sync_enabled, cover_key, updated_at, connection_id)
SELECT 'synology_photos', s.album_id, s.album_id, 'album', 1, '', CURRENT_TIMESTAMP, c.id
FROM connections c,
     (SELECT COALESCE((SELECT value FROM settings WHERE key = 'synology_album_id'), '') AS album_id) s
WHERE c.source = 'synology_photos'
  AND NOT EXISTS (SELECT 1 FROM albums WHERE source = 'synology_photos')
  AND s.album_id != '';

-- Uniqueness moves to (connection_id, external_id). Rows without a connection
-- keep the old (source, external_id) rule through partial indexes.
DROP INDEX IF EXISTS idx_albums_source_external;
CREATE UNIQUE INDEX idx_albums_source_external ON albums(source, external_id)
    WHERE connection_id IS NULL;
CREATE UNIQUE INDEX idx_albums_connection_external ON albums(connection_id, external_id)
    WHERE connection_id IS NOT NULL;
CREATE INDEX idx_albums_connection ON albums(connection_id);

DROP INDEX IF EXISTS idx_images_source_external_unique;
CREATE UNIQUE INDEX idx_images_source_external_unique ON images(source, external_id)
    WHERE external_id != '' AND deleted_at IS NULL AND connection_id IS NULL;
CREATE UNIQUE INDEX idx_images_connection_external_unique ON images(connection_id, external_id)
    WHERE external_id != '' AND deleted_at IS NULL AND connection_id IS NOT NULL;
CREATE INDEX idx_images_connection ON images(connection_id);

-- The moved keys go, so the old UI binding can never resurrect a stale
-- credential and GET /api/settings stops exposing them.
DELETE FROM settings WHERE key IN (
    'immich_url', 'immich_api_key', 'immich_source_mode', 'immich_album_id',
    'immich_memory_mode', 'immich_auto_sync_enabled', 'immich_auto_sync_interval_minutes',
    'synology_url', 'synology_account', 'synology_password', 'synology_skip_cert',
    'synology_album_id', 'synology_auto_sync_enabled', 'synology_auto_sync_interval_minutes',
    'synology_sid', 'synology_did', 'synology_token', 'synology_albums_cache',
    'image_source'
);
