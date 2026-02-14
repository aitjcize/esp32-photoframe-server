CREATE TABLE IF NOT EXISTS google_calendar_auths (
    id INTEGER PRIMARY KEY,
    access_token TEXT NOT NULL DEFAULT '',
    refresh_token TEXT NOT NULL DEFAULT '',
    expiry DATETIME
);

ALTER TABLE devices ADD COLUMN calendar_id TEXT NOT NULL DEFAULT '';
