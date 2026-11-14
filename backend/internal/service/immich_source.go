package service

import (
	"bytes"
	"image"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/imagesource"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// immichSource is the registry plugin for Immich-hosted photos. One plugin
// serves every Immich connection: the device's albums may span several
// servers, and each picked image names the connection to download it from.
type immichSource struct {
	db    *gorm.DB
	conns *ConnectionManager
}

// NewImmichSource constructs the plugin.
func NewImmichSource(db *gorm.DB, conns *ConnectionManager) imagesource.Source {
	return &immichSource{db: db, conns: conns}
}

func (s *immichSource) Name() string { return model.SourceImmich }

func (s *immichSource) Fetch(req *imagesource.Request) (*imagesource.Response, error) {
	// Restrict to the device's bound albums; empty → whole Immich pool.
	var albumIDs []uint
	if req.Device != nil {
		albumIDs = DeviceAlbumIDs(s.db, req.Device.ID, model.SourceImmich)
	}
	pick := func(orientation string, exclude []uint) (model.Image, error) {
		return PickRandomDBPhotoForAlbums(s.db, model.SourceImmich, orientation, albumIDs, exclude)
	}
	load := func(item model.Image) (image.Image, error) {
		svc, err := s.conns.ImmichForImage(item)
		if err != nil {
			return nil, err
		}
		data, err := svc.DownloadPhoto(item.ExternalID)
		if err != nil {
			return nil, err
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		return img, err
	}
	return RunDBPhotoFlow(req, s.db, pick, load)
}
