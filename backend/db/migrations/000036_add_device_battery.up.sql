-- Last battery level the frame reported (X-Battery-Percentage header on
-- image fetches). -1 = never reported.
ALTER TABLE devices ADD COLUMN battery_level INTEGER DEFAULT -1;
ALTER TABLE devices ADD COLUMN battery_reported_at TIMESTAMP;
