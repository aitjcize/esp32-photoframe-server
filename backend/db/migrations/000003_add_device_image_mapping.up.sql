CREATE TABLE IF NOT EXISTS url_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL,
    created_at DATETIME
);

CREATE TABLE IF NOT EXISTS device_url_mappings (
    device_id INTEGER,
    url_source_id INTEGER,
    PRIMARY KEY (device_id, url_source_id)
);
