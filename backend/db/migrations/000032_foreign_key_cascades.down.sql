-- Revert to the original constraint-free junction/child tables. Data-preserving:
-- rebuild each table without FOREIGN KEY clauses and copy every row back.

CREATE TABLE image_album_memberships_new (
    image_id INTEGER NOT NULL,
    album_id INTEGER NOT NULL,
    PRIMARY KEY (image_id, album_id)
);
INSERT INTO image_album_memberships_new SELECT image_id, album_id FROM image_album_memberships;
DROP TABLE image_album_memberships;
ALTER TABLE image_album_memberships_new RENAME TO image_album_memberships;
CREATE INDEX idx_iam_album ON image_album_memberships(album_id);

CREATE TABLE device_album_mappings_new (
    device_id INTEGER NOT NULL,
    album_id INTEGER NOT NULL,
    PRIMARY KEY (device_id, album_id)
);
INSERT INTO device_album_mappings_new SELECT device_id, album_id FROM device_album_mappings;
DROP TABLE device_album_mappings;
ALTER TABLE device_album_mappings_new RENAME TO device_album_mappings;
CREATE INDEX idx_dam_album ON device_album_mappings(album_id);

CREATE TABLE device_url_mappings_new (
    device_id INTEGER,
    url_source_id INTEGER,
    PRIMARY KEY (device_id, url_source_id)
);
INSERT INTO device_url_mappings_new SELECT device_id, url_source_id FROM device_url_mappings;
DROP TABLE device_url_mappings;
ALTER TABLE device_url_mappings_new RENAME TO device_url_mappings;

CREATE TABLE generative_states_new (
    device_id INTEGER NOT NULL,
    source TEXT NOT NULL,
    state BLOB,
    updated_at DATETIME,
    PRIMARY KEY (device_id, source)
);
INSERT INTO generative_states_new SELECT device_id, source, state, updated_at FROM generative_states;
DROP TABLE generative_states;
ALTER TABLE generative_states_new RENAME TO generative_states;

CREATE TABLE device_histories_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id INTEGER,
    image_id INTEGER,
    served_at DATETIME
);
INSERT INTO device_histories_new (id, device_id, image_id, served_at)
    SELECT id, device_id, image_id, served_at FROM device_histories;
DROP TABLE device_histories;
ALTER TABLE device_histories_new RENAME TO device_histories;
CREATE INDEX idx_device_histories_device_id ON device_histories(device_id);
CREATE INDEX idx_device_histories_device_served ON device_histories(device_id, served_at DESC);
