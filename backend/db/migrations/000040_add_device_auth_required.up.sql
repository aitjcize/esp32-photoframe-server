-- Whether the frame refused this server's last request for want of the
-- right password (a 401 on its own HTTP API), and when that first happened.
-- Cleared by the next request the frame answers. Lets the webapp ask for
-- the frame's password instead of every sync and push failing silently.
ALTER TABLE devices ADD COLUMN auth_required BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE devices ADD COLUMN auth_failed_at TIMESTAMP;
