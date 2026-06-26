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

// SearchMetadataRequest is the body for POST /api/search/metadata. Only the
// fields we use are declared; Immich ignores unknown JSON keys on input.
type SearchMetadataRequest struct {
	Type        string `json:"type,omitempty"`       // "IMAGE" — we filter out videos
	IsFavorite  *bool  `json:"isFavorite,omitempty"` // pointer so we can send false vs unset
	TakenAfter  string `json:"takenAfter,omitempty"` // RFC3339
	TakenBefore string `json:"takenBefore,omitempty"`
	Page        int    `json:"page,omitempty"`
	Size        int    `json:"size,omitempty"`
}

// searchAssetsResponse matches /api/search/metadata's "assets" envelope.
type searchAssetsResponse struct {
	Assets struct {
		Count    int     `json:"count"`
		Total    int     `json:"total"`
		NextPage *string `json:"nextPage"`
		Items    []Asset `json:"items"`
	} `json:"assets"`
}

// MemoryLane is one "on this day" group returned by GET /api/memories.
// Each lane corresponds to a single past year; Data.Year identifies it.
type MemoryLane struct {
	ID     string  `json:"id"`
	Assets []Asset `json:"assets"`
	Data   struct {
		Year int `json:"year"`
	} `json:"data"`
}
