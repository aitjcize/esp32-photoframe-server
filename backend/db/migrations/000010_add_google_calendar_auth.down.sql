DROP TABLE IF EXISTS google_calendar_auths;

-- SQLite doesn't support DROP COLUMN, so we recreate the table without calendar_id
-- In practice, the column is harmless to leave. This is a best-effort rollback.
