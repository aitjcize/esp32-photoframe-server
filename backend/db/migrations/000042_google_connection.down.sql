-- Lossy by design, like 000041: the single-token schema keeps the token of the
-- lowest-id Google connection only. Every Google Photos image is kept (they are
-- local files that need no account to serve) and detached from its connection
-- before the connection rows go, so the ON DELETE CASCADE does not take them.

CREATE TABLE IF NOT EXISTS google_auths (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    access_token TEXT,
    refresh_token TEXT,
    expiry DATETIME
);

INSERT INTO google_auths (id, access_token, refresh_token, expiry)
SELECT 1,
    json_extract(state, '$.access_token'),
    json_extract(state, '$.refresh_token'),
    json_extract(state, '$.expiry')
FROM connections
WHERE source = 'google_photos' AND json_extract(state, '$.refresh_token') IS NOT NULL
ORDER BY id LIMIT 1;

UPDATE images SET connection_id = NULL WHERE source = 'google_photos';

DELETE FROM connections WHERE source = 'google_photos';
