-- Google Calendar becomes a connection-backed account like Google Photos
-- (000042): one connections row per Google account whose calendar frames can
-- show, with the OAuth token in the row's state column. The single token in
-- google_calendar_auths moves into one such row, and every frame that shows a
-- calendar is pointed at it through the new devices.calendar_connection_id
-- (NULL means "the first connected account").

ALTER TABLE devices ADD COLUMN calendar_connection_id INTEGER REFERENCES connections(id) ON DELETE SET NULL;

INSERT INTO connections (source, name, enabled, config, state, created_at, updated_at)
SELECT 'google_calendar', 'Google Calendar', 1, '{}',
    json_object(
        'access_token', COALESCE(access_token, ''),
        'refresh_token', COALESCE(refresh_token, ''),
        'expiry', strftime('%Y-%m-%dT%H:%M:%fZ', expiry)
    ),
    CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM google_calendar_auths
WHERE COALESCE(refresh_token, '') <> '' OR COALESCE(access_token, '') <> ''
ORDER BY id LIMIT 1;

UPDATE devices
SET calendar_connection_id = (SELECT id FROM connections WHERE source = 'google_calendar' ORDER BY id LIMIT 1)
WHERE show_calendar = 1
  AND EXISTS (SELECT 1 FROM connections WHERE source = 'google_calendar');

DROP TABLE IF EXISTS google_calendar_auths;
