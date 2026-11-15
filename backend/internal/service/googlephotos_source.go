package service

import (
	"image"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/imagesource"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// googlePhotosSource is the registry plugin for Google Photos. The picker
// service downloads photos to local disk during session import; here we
// only need to read them back from the same dataDir as the gallery source.
type googlePhotosSource struct {
	db      *gorm.DB
	dataDir string
}

// NewGooglePhotosSource constructs the plugin.
func NewGooglePhotosSource(db *gorm.DB, dataDir string) imagesource.Source {
	return &googlePhotosSource{db: db, dataDir: dataDir}
}

func (s *googlePhotosSource) Name() string { return model.SourceGooglePhotos }

func (s *googlePhotosSource) Fetch(req *imagesource.Request) (*imagesource.Response, error) {
	pick := func(orientation string, exclude []uint) (model.Image, error) {
		return PickRandomDBPhotoForDevice(s.db, model.SourceGooglePhotos, req.Device, orientation, exclude)
	}
	load := func(item model.Image) (image.Image, error) {
		return LoadLocalPhoto(s.dataDir, item)
	}
	return RunDBPhotoFlow(req, s.db, pick, load)
}
