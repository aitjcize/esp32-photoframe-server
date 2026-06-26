-- Per-device image source, used by the unified /image endpoint so the source
-- is resolved server-side from the requesting device instead of the URL path.
ALTER TABLE devices ADD COLUMN source TEXT DEFAULT '';
