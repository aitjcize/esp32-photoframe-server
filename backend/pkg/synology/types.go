package synology

type BrowseItemResponse struct {
	Success bool `json:"success"`
	Data    struct {
		List []Item `json:"list"`
	} `json:"data"`
	Error struct {
		Code int `json:"code"`
	} `json:"error"`
}

type Item struct {
	ID          int    `json:"id"`
	Filename    string `json:"filename"`
	Filesize    int    `json:"filesize"`
	Time        int64  `json:"time"`
	IndexedTime int64  `json:"indexed_time"`
	OwnerUserID int    `json:"owner_user_id"`
	FolderID    int    `json:"folder_id"`
	Type        string `json:"type"` // "photo" or "video"
	Additional  struct {
		Thumbnail struct {
			M  string `json:"m"` // Cache key or similar
			XL string `json:"xl"`
			S  string `json:"s"`
		} `json:"thumbnail"`
		Resolution struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"resolution"`
	} `json:"additional"`
}

type BrowseAlbumResponse struct {
	Success bool `json:"success"`
	Data    struct {
		List []Album `json:"list"`
	} `json:"data"`
	Error struct {
		Code int `json:"code"`
	} `json:"error"`
}

type Album struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "folder" or "album"
}
