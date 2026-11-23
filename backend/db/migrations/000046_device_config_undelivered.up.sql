-- Whether the frame's settings stored on the server have not reached the
-- frame yet: the direct push after an edit failed (frame asleep, or it
-- refused the password), and the frame has not picked them up since. Saving
-- the same settings again then retries the push instead of skipping it.
ALTER TABLE devices ADD COLUMN config_undelivered BOOLEAN NOT NULL DEFAULT 0;
-- Nothing recorded whether stored settings were delivered before this, so
-- they count as unconfirmed until the frame reports them or a push lands.
UPDATE devices SET config_undelivered = 1 WHERE config_last_updated > 0;
