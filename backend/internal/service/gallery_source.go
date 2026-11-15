package service

import (
	"image"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/imagesource"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// gallerySource is the registry plugin for the local gallery (photos
// uploaded directly to the server and stored on disk).
type gallerySource struct {
	db      *gorm.DB
	dataDir string
}

// NewGallerySource constructs the plugin.
func NewGallerySource(db *gorm.DB, dataDir string) imagesource.Source {
	return &gallerySource{db: db, dataDir: dataDir}
}

func (s *gallerySource) Name() string { return model.SourceGallery }

func (s *gallerySource) Fetch(req *imagesource.Request) (*imagesource.Response, error) {
	// A frame bound to gallery albums draws from those; one without a
	// binding draws from the whole gallery, unfiled photos included.
	var albumIDs []uint
	if req.Device != nil {
		albumIDs = DeviceAlbumIDs(s.db, req.Device.ID, model.SourceGallery)
	}
	pick := func(orientation string, exclude []uint) (model.Image, error) {
		return PickRandomDBPhotoForAlbums(s.db, model.SourceGallery, orientation, albumIDs, exclude)
	}
	load := func(item model.Image) (image.Image, error) {
		return LoadLocalPhoto(s.dataDir, item)
	}
	return RunDBPhotoFlow(req, s.db, pick, load)
}
