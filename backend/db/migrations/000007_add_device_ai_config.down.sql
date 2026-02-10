-- SQLite does not support DROP COLUMN before 3.35.0
-- Use a table rebuild approach for broader compatibility
CREATE TABLE devices_backup AS SELECT id, name, host, width, height, use_device_parameter, orientation, enable_collage, show_date, show_weather, weather_lat, weather_lon, created_at FROM devices;
DROP TABLE devices;
ALTER TABLE devices_backup RENAME TO devices;
