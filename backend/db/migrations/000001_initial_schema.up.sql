CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE,
    password TEXT,
    created_at DATETIME,
    updated_at DATETIME
);

CREATE TABLE IF NOT EXISTS api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    name TEXT,
    created_at DATETIME
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT
);

CREATE TABLE IF NOT EXISTS images (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_path TEXT,
    caption TEXT,
    width INTEGER,
    height INTEGER,
    orientation TEXT,
    user_id INTEGER,
    status TEXT,
    source TEXT,
    synology_photo_id INTEGER,
    synology_space TEXT,
    thumbnail_key TEXT,
    created_at DATETIME,
    deleted_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_images_deleted_at ON images(deleted_at);

CREATE TABLE IF NOT EXISTS google_auths (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    access_token TEXT,
    refresh_token TEXT,
    expiry DATETIME
);

CREATE TABLE IF NOT EXISTS devices (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT,
    host TEXT,
    width INTEGER DEFAULT 800,
    height INTEGER DEFAULT 480,
    use_device_parameter BOOLEAN DEFAULT FALSE,
    orientation TEXT DEFAULT 'landscape',
    enable_collage BOOLEAN DEFAULT FALSE,
    show_date BOOLEAN DEFAULT FALSE,
    show_weather BOOLEAN DEFAULT FALSE,
    weather_lat REAL DEFAULT 0,
    weather_lon REAL DEFAULT 0,
    created_at DATETIME
);
