-- SQLite does not support DROP COLUMN directly in older versions.
-- We leave these columns in place on rollback.
-- To fully revert, recreate the tables without these columns.
SELECT 1;
