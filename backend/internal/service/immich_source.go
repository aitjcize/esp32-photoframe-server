package service

import (
	"bytes"
	"image"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/imagesource"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// immichSource is the registry plugin for Immich-hosted photos.
type immichSource struct {
	db     *gorm.DB
	immich *ImmichService
}

// NewImmichSource constructs the plugin.
func NewImmichSource(db *gorm.DB, immich *ImmichService) imagesource.Source {
	return &immichSource{db: db, immich: immich}
}

func (s *immichSource) Name() string { return model.SourceImmich }

func (s *immichSource) Fetch(req *imagesource.Request) (*imagesource.Response, error) {
	var albumIDs []uint
	if req.Device != nil {
		albumIDs = DeviceAlbumIDs(s.db, req.Device.ID, model.SourceImmich)
	}
	dateFrom, dateTo := s.immich.DateRange()
	pick := func(orientation string, exclude []uint) (model.Image, error) {
		return PickRandomDBPhotoForAlbumsFiltered(s.db, model.SourceImmich, orientation, albumIDs, exclude, dateFrom, dateTo)
	}
	load := func(item model.Image) (image.Image, error) {
		data, err := s.immich.DownloadPhoto(item.ExternalID)
		if err != nil {
			return nil, err
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		return img, err
	}
	return RunDBPhotoFlow(req, s.db, pick, load)
}
