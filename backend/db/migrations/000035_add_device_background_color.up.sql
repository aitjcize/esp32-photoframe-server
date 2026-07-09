-- Background palette color used to letterbox photos in fit display mode
-- (passed to epaper-image-convert --background-color). Empty = CLI default
-- (white).
ALTER TABLE devices ADD COLUMN background_color TEXT DEFAULT '';
