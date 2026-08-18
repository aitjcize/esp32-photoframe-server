-- Last firmware version the frame reported (X-Firmware-Version header on
-- image fetches). Empty = never reported.
ALTER TABLE devices ADD COLUMN firmware_version TEXT DEFAULT '';
