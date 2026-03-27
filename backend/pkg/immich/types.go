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
	DateTimeOriginal string `json:"dateTimeOriginal"` // ISO 8601 e.g. "2024-06-01T10:30:00.000Z"
}

// Asset represents an Immich media asset
type Asset struct {
	ID               string   `json:"id"`
	Type             string   `json:"type"` // "IMAGE", "VIDEO"
	OriginalFileName string   `json:"originalFileName"`
	FileCreatedAt    string   `json:"fileCreatedAt"` // ISO 8601 date when file was created
	LocalDateTime    string   `json:"localDateTime"` // Local datetime of photo
	ExifInfo         ExifInfo `json:"exifInfo"`
}

// AlbumDetail is the full album response including assets
type AlbumDetail struct {
	ID        string  `json:"id"`
	AlbumName string  `json:"albumName"`
	Assets    []Asset `json:"assets"`
}
