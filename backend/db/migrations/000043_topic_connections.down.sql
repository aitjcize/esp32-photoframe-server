-- Lossy by design, like 000041: the settings-key schema holds one API key per
-- source, so the lowest-id Unsplash and Pexels connections are written back to
-- the settings keys and keep their topics and photos (detached), while the
-- other connections' rows go with them.

INSERT OR REPLACE INTO settings (key, value)
SELECT 'unsplash_api_key', json_extract(config, '$.api_key') FROM connections WHERE source = 'unsplash' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'unsplash_randomize_results', CASE WHEN json_extract(config, '$.randomize_results') THEN 'true' ELSE 'false' END
FROM connections WHERE source = 'unsplash' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'unsplash_auto_sync_enabled', CASE WHEN json_extract(config, '$.auto_sync_enabled') THEN 'true' ELSE 'false' END
FROM connections WHERE source = 'unsplash' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'unsplash_auto_sync_interval', CAST(json_extract(config, '$.auto_sync_interval_minutes') AS TEXT)
FROM connections WHERE source = 'unsplash' ORDER BY id LIMIT 1;

INSERT OR REPLACE INTO settings (key, value)
SELECT 'pexels_api_key', json_extract(config, '$.api_key') FROM connections WHERE source = 'pexels' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'pexels_randomize_results', CASE WHEN json_extract(config, '$.randomize_results') THEN 'true' ELSE 'false' END
FROM connections WHERE source = 'pexels' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'pexels_auto_sync_enabled', CASE WHEN json_extract(config, '$.auto_sync_enabled') THEN 'true' ELSE 'false' END
FROM connections WHERE source = 'pexels' ORDER BY id LIMIT 1;
INSERT OR REPLACE INTO settings (key, value)
SELECT 'pexels_auto_sync_interval', CAST(json_extract(config, '$.auto_sync_interval_minutes') AS TEXT)
FROM connections WHERE source = 'pexels' ORDER BY id LIMIT 1;

-- Keep only the first connection's rows per source; the rest would collide on
-- (source, external_id) once detached.
DELETE FROM albums WHERE source IN ('unsplash', 'pexels') AND connection_id IS NOT NULL AND connection_id NOT IN (
    SELECT MIN(id) FROM connections WHERE source IN ('unsplash', 'pexels') GROUP BY source);
DELETE FROM images WHERE source IN ('unsplash', 'pexels') AND connection_id IS NOT NULL AND connection_id NOT IN (
    SELECT MIN(id) FROM connections WHERE source IN ('unsplash', 'pexels') GROUP BY source);

UPDATE albums SET connection_id = NULL WHERE source IN ('unsplash', 'pexels');
UPDATE images SET connection_id = NULL WHERE source IN ('unsplash', 'pexels');

DELETE FROM connections WHERE source IN ('unsplash', 'pexels');
