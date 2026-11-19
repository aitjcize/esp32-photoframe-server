package service

import (
	"bytes"
	"image"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/imagesource"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// nextcloudSource is the registry plugin for photos synced from Nextcloud
// Memories. One plugin serves every Nextcloud connection; each picked image
// names the connection to download it from.
type nextcloudSource struct {
	db    *gorm.DB
	conns *ConnectionManager
}

// NewNextcloudSource constructs the plugin.
func NewNextcloudSource(db *gorm.DB, conns *ConnectionManager) imagesource.Source {
	return &nextcloudSource{db: db, conns: conns}
}

func (s *nextcloudSource) Name() string { return model.SourceNextcloudMemories }

func (s *nextcloudSource) Fetch(req *imagesource.Request) (*imagesource.Response, error) {
	pick := func(orientation string, exclude []uint) (model.Image, error) {
		return PickRandomDBPhotoForDevice(s.db, model.SourceNextcloudMemories, req.Device, orientation, exclude)
	}
	load := func(item model.Image) (image.Image, error) {
		svc, err := s.conns.NextcloudForImage(item)
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
