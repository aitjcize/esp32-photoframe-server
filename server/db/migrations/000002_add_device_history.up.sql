CREATE TABLE IF NOT EXISTS device_histories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id INTEGER,
    image_id INTEGER,
    served_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_device_histories_device_id ON device_histories(device_id);
