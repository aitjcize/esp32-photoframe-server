-- A frame on a connection-backed source (Immich, Synology, Google Photos,
-- Unsplash, Pexels) can be bound to one connection of that source instead of
-- drawing from all of them: devices.image_connection_id, NULL meaning "any
-- connection of the source", as every frame behaved so far. A frame whose
-- source has exactly one connection is bound to it, which is what its owner
-- sees in the device dialog; a frame whose source has several keeps drawing
-- from all of them until its owner picks one.

ALTER TABLE devices ADD COLUMN image_connection_id INTEGER REFERENCES connections(id) ON DELETE SET NULL;

UPDATE devices
SET image_connection_id = (SELECT c.id FROM connections c WHERE c.source = devices.source)
WHERE source IN (SELECT source FROM connections GROUP BY source HAVING COUNT(*) = 1);
