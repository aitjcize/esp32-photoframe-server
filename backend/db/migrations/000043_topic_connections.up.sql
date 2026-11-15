-- Unsplash and Pexels become connection-backed sources like Immich, Synology
-- and Google Photos (000041, 000042): one connections row per API key. The
-- flat unsplash_*/pexels_* settings move into that row's config, and the
-- source's topic albums and photos are reparented to it, so a second key (a
-- second Unsplash account, say) can hold its own topics.

INSERT INTO connections (source, name, enabled, config, state, created_at, updated_at)
SELECT 'unsplash', 'Unsplash',
    CASE WHEN COALESCE((SELECT value FROM settings WHERE key = 'unsplash_api_key'), '') != ''
         THEN 1 ELSE 0 END,
    json_object(
        'api_key', COALESCE((SELECT value FROM settings WHERE key = 'unsplash_api_key'), ''),
        'randomize_results', json(CASE WHEN lower(COALESCE((SELECT value FROM settings WHERE key = 'unsplash_randomize_results'), '')) = 'true'
                                       THEN 'true' ELSE 'false' END),
        'auto_sync_enabled', json(CASE WHEN lower(COALESCE((SELECT value FROM settings WHERE key = 'unsplash_auto_sync_enabled'), '')) = 'true'
                                       THEN 'true' ELSE 'false' END),
        'auto_sync_interval_minutes', CAST(COALESCE(NULLIF((SELECT value FROM settings WHERE key = 'unsplash_auto_sync_interval'), ''), '60') AS INTEGER)
    ),
    '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE COALESCE((SELECT value FROM settings WHERE key = 'unsplash_api_key'), '') != ''
   OR EXISTS (SELECT 1 FROM albums WHERE source = 'unsplash' AND connection_id IS NULL)
   OR EXISTS (SELECT 1 FROM images WHERE source = 'unsplash' AND connection_id IS NULL);

INSERT INTO connections (source, name, enabled, config, state, created_at, updated_at)
SELECT 'pexels', 'Pexels',
    CASE WHEN COALESCE((SELECT value FROM settings WHERE key = 'pexels_api_key'), '') != ''
         THEN 1 ELSE 0 END,
    json_object(
        'api_key', COALESCE((SELECT value FROM settings WHERE key = 'pexels_api_key'), ''),
        'randomize_results', json(CASE WHEN lower(COALESCE((SELECT value FROM settings WHERE key = 'pexels_randomize_results'), '')) = 'true'
                                       THEN 'true' ELSE 'false' END),
        'auto_sync_enabled', json(CASE WHEN lower(COALESCE((SELECT value FROM settings WHERE key = 'pexels_auto_sync_enabled'), '')) = 'true'
                                       THEN 'true' ELSE 'false' END),
        'auto_sync_interval_minutes', CAST(COALESCE(NULLIF((SELECT value FROM settings WHERE key = 'pexels_auto_sync_interval'), ''), '60') AS INTEGER)
    ),
    '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE COALESCE((SELECT value FROM settings WHERE key = 'pexels_api_key'), '') != ''
   OR EXISTS (SELECT 1 FROM albums WHERE source = 'pexels' AND connection_id IS NULL)
   OR EXISTS (SELECT 1 FROM images WHERE source = 'pexels' AND connection_id IS NULL);

UPDATE albums
SET connection_id = (SELECT id FROM connections WHERE connections.source = albums.source ORDER BY id LIMIT 1)
WHERE source IN ('unsplash', 'pexels') AND connection_id IS NULL;
UPDATE images
SET connection_id = (SELECT id FROM connections WHERE connections.source = images.source ORDER BY id LIMIT 1)
WHERE source IN ('unsplash', 'pexels') AND connection_id IS NULL;

DELETE FROM settings WHERE key IN (
    'unsplash_api_key', 'unsplash_randomize_results', 'unsplash_auto_sync_enabled', 'unsplash_auto_sync_interval',
    'pexels_api_key', 'pexels_randomize_results', 'pexels_auto_sync_enabled', 'pexels_auto_sync_interval'
);
