-- Add real foreign keys with ON DELETE actions to the junction/child tables so
-- referential integrity is enforced by the DB instead of by hand-written cleanup
-- scattered across the Go handlers (DeleteDevice, DeletePhoto, clearSourcePhotos,
-- SetSyncTopics, ...). Paired with _foreign_keys=on in the connection DSN.
--
-- SQLite can't ALTER TABLE ADD CONSTRAINT, so each table is rebuilt (create new,
-- copy, drop, rename). This runs with foreign_keys=ON (enforcement is enabled in
-- the DSN), so the copy must already satisfy the constraints: the INSERT..SELECT
-- for each table copies only rows whose parents still exist, which purges any
-- pre-existing orphans in the same step. All five are leaf tables (nothing
-- references them), so dropping/renaming can't violate an inbound FK.
--
-- ON DELETE actions:
--   * CASCADE everywhere a child row is meaningless without its parent (a
--     membership without its image/album, a mapping without its device/album/
--     url-source, a generative state without its device, a history row without
--     its device).
--   * SET NULL for device_histories.image_id — the "what was served when" log is
--     worth keeping after the photo is deleted; just drop the dangling reference.

-- image_album_memberships: image_id, album_id both CASCADE
CREATE TABLE image_album_memberships_new (
    image_id INTEGER NOT NULL,
    album_id INTEGER NOT NULL,
    PRIMARY KEY (image_id, album_id),
    FOREIGN KEY (image_id) REFERENCES images(id) ON DELETE CASCADE,
    FOREIGN KEY (album_id) REFERENCES albums(id) ON DELETE CASCADE
);
INSERT INTO image_album_memberships_new (image_id, album_id)
    SELECT image_id, album_id FROM image_album_memberships
    WHERE image_id IN (SELECT id FROM images)
      AND album_id IN (SELECT id FROM albums);
DROP TABLE image_album_memberships;
ALTER TABLE image_album_memberships_new RENAME TO image_album_memberships;
CREATE INDEX idx_iam_album ON image_album_memberships(album_id);

-- device_album_mappings: device_id, album_id both CASCADE
CREATE TABLE device_album_mappings_new (
    device_id INTEGER NOT NULL,
    album_id INTEGER NOT NULL,
    PRIMARY KEY (device_id, album_id),
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE,
    FOREIGN KEY (album_id) REFERENCES albums(id) ON DELETE CASCADE
);
INSERT INTO device_album_mappings_new (device_id, album_id)
    SELECT device_id, album_id FROM device_album_mappings
    WHERE device_id IN (SELECT id FROM devices)
      AND album_id IN (SELECT id FROM albums);
DROP TABLE device_album_mappings;
ALTER TABLE device_album_mappings_new RENAME TO device_album_mappings;
CREATE INDEX idx_dam_album ON device_album_mappings(album_id);

-- device_url_mappings: device_id, url_source_id both CASCADE
CREATE TABLE device_url_mappings_new (
    device_id INTEGER,
    url_source_id INTEGER,
    PRIMARY KEY (device_id, url_source_id),
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE,
    FOREIGN KEY (url_source_id) REFERENCES url_sources(id) ON DELETE CASCADE
);
INSERT INTO device_url_mappings_new (device_id, url_source_id)
    SELECT device_id, url_source_id FROM device_url_mappings
    WHERE device_id IN (SELECT id FROM devices)
      AND url_source_id IN (SELECT id FROM url_sources);
DROP TABLE device_url_mappings;
ALTER TABLE device_url_mappings_new RENAME TO device_url_mappings;

-- generative_states: device_id CASCADE
CREATE TABLE generative_states_new (
    device_id INTEGER NOT NULL,
    source TEXT NOT NULL,
    state BLOB,
    updated_at DATETIME,
    PRIMARY KEY (device_id, source),
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
);
INSERT INTO generative_states_new (device_id, source, state, updated_at)
    SELECT device_id, source, state, updated_at FROM generative_states
    WHERE device_id IN (SELECT id FROM devices);
DROP TABLE generative_states;
ALTER TABLE generative_states_new RENAME TO generative_states;

-- device_histories: device_id CASCADE, image_id SET NULL
CREATE TABLE device_histories_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id INTEGER,
    image_id INTEGER,
    served_at DATETIME,
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE,
    FOREIGN KEY (image_id) REFERENCES images(id) ON DELETE SET NULL
);
INSERT INTO device_histories_new (id, device_id, image_id, served_at)
    SELECT id, device_id,
           CASE WHEN image_id IN (SELECT id FROM images) THEN image_id ELSE NULL END,
           served_at
    FROM device_histories
    WHERE device_id IS NULL OR device_id IN (SELECT id FROM devices);
DROP TABLE device_histories;
ALTER TABLE device_histories_new RENAME TO device_histories;
CREATE INDEX idx_device_histories_device_id ON device_histories(device_id);
CREATE INDEX idx_device_histories_device_served ON device_histories(device_id, served_at DESC);
