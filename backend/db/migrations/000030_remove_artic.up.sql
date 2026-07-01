-- Remove the ARTIC source entirely: its topic albums, images, album
-- memberships, per-device bindings, and settings. The integration was dropped.
DELETE FROM image_album_memberships WHERE album_id IN (SELECT id FROM albums WHERE source = 'artic');
DELETE FROM device_album_mappings WHERE album_id IN (SELECT id FROM albums WHERE source = 'artic');
DELETE FROM images WHERE source = 'artic';
DELETE FROM albums WHERE source = 'artic';
DELETE FROM settings WHERE key LIKE 'artic%';
