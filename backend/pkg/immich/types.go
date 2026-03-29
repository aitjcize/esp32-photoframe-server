package immich

// Album represents an Immich album
type Album struct {
	ID         string `json:"id"`
	AlbumName  string `json:"albumName"`
	AssetCount int    `json:"assetCount"`
}

// ExifInfo holds EXIF metadata for an asset
type ExifInfo struct {
	ExifImageWidth   int    `json:"exifImageWidth"`
	ExifImageHeight  int    `json:"exifImageHeight"`
	Orientation      string `json:"orientation"` // EXIF orientation e.g. "1", "6", "Rotate 90 CW"
	DateTimeOriginal string `json:"dateTimeOriginal"`
}

// Asset represents an Immich media asset
type Asset struct {
	ID               string   `json:"id"`
	Type             string   `json:"type"` // "IMAGE", "VIDEO"
	OriginalFileName string   `json:"originalFileName"`
	LocalDateTime    string   `json:"localDateTime"`
	ExifInfo         ExifInfo `json:"exifInfo"`
}

// AlbumDetail is the full album response including assets
type AlbumDetail struct {
	ID        string  `json:"id"`
	AlbumName string  `json:"albumName"`
	Assets    []Asset `json:"assets"`
}
