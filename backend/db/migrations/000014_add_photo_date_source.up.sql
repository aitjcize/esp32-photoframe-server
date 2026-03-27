-- Add DateSource to devices table (controls whether to show current date or photo creation date)
ALTER TABLE devices ADD COLUMN date_source TEXT NOT NULL DEFAULT 'current';

-- Add PhotoTakenAt to images table (stores original photo creation/taken date from EXIF or API metadata)
ALTER TABLE images ADD COLUMN photo_taken_at DATETIME;
