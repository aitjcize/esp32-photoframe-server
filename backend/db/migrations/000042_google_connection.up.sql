-- Google Photos becomes a connection-backed source like Immich and Synology
-- (000041): one connections row per Google account. The single OAuth token in
-- google_auths moves into that row's state column and the Google Photos images
-- are reparented to it. The OAuth client id and secret stay global settings:
-- one Google Cloud project authorises any number of accounts.

-- One connection when there is anything to carry over: a token, or imported
-- photos (the account may have been disconnected, the photos stay usable).
-- The expiry is rewritten as RFC 3339 UTC so Go's time.Time decodes it.
INSERT INTO connections (source, name, enabled, config, state, created_at, updated_at)
SELECT 'google_photos', 'Google Photos', 1, '{}',
    COALESCE((
        SELECT json_object(
            'access_token', COALESCE(access_token, ''),
            'refresh_token', COALESCE(refresh_token, ''),
            'expiry', strftime('%Y-%m-%dT%H:%M:%fZ', expiry)
        )
        FROM google_auths ORDER BY id LIMIT 1
    ), '{}'),
    CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
WHERE EXISTS (SELECT 1 FROM google_auths)
   OR EXISTS (SELECT 1 FROM images WHERE source = 'google_photos' AND connection_id IS NULL);

UPDATE images
SET connection_id = (SELECT id FROM connections WHERE source = 'google_photos' ORDER BY id LIMIT 1)
WHERE source = 'google_photos' AND connection_id IS NULL;

DROP TABLE IF EXISTS google_auths;
