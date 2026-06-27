-- Drop the cached albums.asset_count column. It went stale whenever images
-- were removed outside a sync, so ListAlbums now computes the count live from
-- memberships (joined to existing images). The column is dead data.
ALTER TABLE albums DROP COLUMN asset_count;
