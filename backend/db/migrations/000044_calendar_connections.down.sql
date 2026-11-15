-- Lossy by design, like 000042: the single-token schema keeps the token of the
-- lowest-id Google Calendar connection only. SQLite cannot drop a column that
-- carries a REFERENCES clause, so devices is rebuilt without
-- calendar_connection_id; its child tables are saved first, because dropping
-- a parent table runs their ON DELETE actions, and recreated afterwards.

CREATE TABLE IF NOT EXISTS google_calendar_auths (
    id INTEGER PRIMARY KEY,
    access_token TEXT NOT NULL DEFAULT '',
    refresh_token TEXT NOT NULL DEFAULT '',
    expiry DATETIME
);

INSERT INTO google_calendar_auths (id, access_token, refresh_token, expiry)
SELECT 1,
    COALESCE(json_extract(state, '$.access_token'), ''),
    COALESCE(json_extract(state, '$.refresh_token'), ''),
    json_extract(state, '$.expiry')
FROM connections
WHERE source = 'google_calendar' AND json_extract(state, '$.refresh_token') IS NOT NULL
ORDER BY id LIMIT 1;

-- Save the children of devices, and the AUTOINCREMENT counters of the rebuilt
-- tables: a rebuilt table restarts at its highest surviving id, and a reissued
-- api_keys id would bring a revoked device token back to life.
CREATE TABLE seq_backup AS SELECT name, seq FROM sqlite_sequence
    WHERE name IN ('devices', 'api_keys', 'device_histories');
CREATE TABLE ak_backup AS SELECT id, user_id, name, created_at, device_id FROM api_keys;
CREATE TABLE dam_backup AS SELECT device_id, album_id FROM device_album_mappings;
CREATE TABLE dum_backup AS SELECT device_id, url_source_id FROM device_url_mappings;
CREATE TABLE gs_backup AS SELECT device_id, source, state, updated_at FROM generative_states;
CREATE TABLE dh_backup AS SELECT id, device_id, image_id, served_at FROM device_histories;
DROP TABLE api_keys;
DROP TABLE device_album_mappings;
DROP TABLE device_url_mappings;
DROP TABLE generative_states;
DROP TABLE device_histories;

-- devices without calendar_connection_id (the 000043 shape)
CREATE TABLE devices_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT,
    host TEXT,
    width INTEGER DEFAULT 800,
    height INTEGER DEFAULT 480,
    orientation TEXT DEFAULT 'landscape',
    enable_collage BOOLEAN DEFAULT FALSE,
    show_date BOOLEAN DEFAULT FALSE,
    show_weather BOOLEAN DEFAULT FALSE,
    weather_lat REAL DEFAULT 0,
    weather_lon REAL DEFAULT 0,
    created_at DATETIME,
    ai_provider TEXT DEFAULT '',
    ai_model TEXT DEFAULT '',
    ai_prompt TEXT DEFAULT '',
    layout TEXT DEFAULT 'photo_overlay',
    show_calendar BOOLEAN DEFAULT 0,
    display_mode TEXT DEFAULT 'cover',
    calendar_id TEXT NOT NULL DEFAULT '',
    date_format TEXT NOT NULL DEFAULT '',
    show_photo_date BOOLEAN NOT NULL DEFAULT 0,
    device_config TEXT DEFAULT '{}',
    device_processing_settings TEXT DEFAULT '{}',
    device_color_palette TEXT DEFAULT '{}',
    config_last_updated BIGINT DEFAULT 0,
    board_name TEXT DEFAULT '',
    source TEXT DEFAULT '',
    display_type TEXT DEFAULT '',
    background_color TEXT DEFAULT '',
    battery_level INTEGER DEFAULT -1,
    battery_reported_at TIMESTAMP,
    firmware_version TEXT DEFAULT '',
    http_password TEXT NOT NULL DEFAULT '',
    auth_required BOOLEAN NOT NULL DEFAULT 0,
    auth_failed_at TIMESTAMP
);
INSERT INTO devices_new (id, name, host, width, height, orientation, enable_collage, show_date, show_weather,
    weather_lat, weather_lon, created_at, ai_provider, ai_model, ai_prompt, layout, show_calendar, display_mode,
    calendar_id, date_format, show_photo_date, device_config, device_processing_settings, device_color_palette,
    config_last_updated, board_name, source, display_type, background_color, battery_level, battery_reported_at,
    firmware_version, http_password, auth_required, auth_failed_at)
  SELECT id, name, host, width, height, orientation, enable_collage, show_date, show_weather,
    weather_lat, weather_lon, created_at, ai_provider, ai_model, ai_prompt, layout, show_calendar, display_mode,
    calendar_id, date_format, show_photo_date, device_config, device_processing_settings, device_color_palette,
    config_last_updated, board_name, source, display_type, background_color, battery_level, battery_reported_at,
    firmware_version, http_password, auth_required, auth_failed_at
  FROM devices;
DROP TABLE devices;
ALTER TABLE devices_new RENAME TO devices;
CREATE INDEX idx_devices_host ON devices(host);

-- The children again, as their migrations made them.
CREATE TABLE api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    name TEXT,
    created_at DATETIME,
    device_id INTEGER REFERENCES devices(id) ON DELETE SET NULL
);
INSERT INTO api_keys (id, user_id, name, created_at, device_id)
    SELECT id, user_id, name, created_at,
           CASE WHEN device_id IN (SELECT id FROM devices) THEN device_id ELSE NULL END
    FROM ak_backup;
CREATE INDEX idx_api_keys_user_device ON api_keys(user_id, device_id);
CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);

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

CREATE TABLE device_url_mappings (
    device_id INTEGER,
    url_source_id INTEGER,
    PRIMARY KEY (device_id, url_source_id),
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE,
    FOREIGN KEY (url_source_id) REFERENCES url_sources(id) ON DELETE CASCADE
);
INSERT INTO device_url_mappings (device_id, url_source_id)
    SELECT device_id, url_source_id FROM dum_backup
    WHERE device_id IN (SELECT id FROM devices) AND url_source_id IN (SELECT id FROM url_sources);

CREATE TABLE generative_states (
    device_id INTEGER NOT NULL,
    source TEXT NOT NULL,
    state BLOB,
    updated_at DATETIME,
    PRIMARY KEY (device_id, source),
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);
INSERT INTO generative_states (device_id, source, state, updated_at)
    SELECT device_id, source, state, updated_at FROM gs_backup
    WHERE device_id IN (SELECT id FROM devices);

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

DELETE FROM sqlite_sequence WHERE name IN ('devices', 'api_keys', 'device_histories');
INSERT INTO sqlite_sequence (name, seq) SELECT name, seq FROM seq_backup;

DROP TABLE seq_backup;
DROP TABLE ak_backup;
DROP TABLE dam_backup;
DROP TABLE dum_backup;
DROP TABLE gs_backup;
DROP TABLE dh_backup;

DELETE FROM connections WHERE source = 'google_calendar';
