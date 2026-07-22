package service

import (
	"bytes"
	"image"
	"os"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/imagesource"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// immichSource is the registry plugin for Immich-hosted photos.
type immichSource struct {
	db     *gorm.DB
	immich *ImmichService
	cache  *ImmichCacheService
}

// NewImmichSource constructs the plugin.
func NewImmichSource(db *gorm.DB, immich *ImmichService, cache *ImmichCacheService) imagesource.Source {
	return &immichSource{db: db, immich: immich, cache: cache}
}

func (s *immichSource) Name() string { return model.SourceImmich }

func (s *immichSource) Fetch(req *imagesource.Request) (*imagesource.Response, error) {
	var albumIDs []uint
	if req.Device != nil {
		albumIDs = DeviceAlbumIDs(s.db, req.Device.ID, model.SourceImmich)
	}
	dateFrom, dateTo := s.immich.DateRange()
	cacheEnabled := s.cache != nil && s.cache.Enabled()
	pick := func(orientation string, exclude []uint) (model.Image, error) {
		return PickRandomDBPhotoForAlbumsFiltered(s.db, model.SourceImmich, orientation, albumIDs, exclude, dateFrom, dateTo, cacheEnabled)
	}
	load := func(item model.Image) (image.Image, error) {
		// Try cache first
		if cacheEnabled {
			if cached := s.cache.Lookup(item.ID); cached != "" {
				img, err := loadLocalImage(cached)
				if err == nil {
					return img, nil
				}
			}
		}

		// Fall back to Immich download
		data, err := s.immich.DownloadPhoto(item.ExternalID)
		if err != nil {
			return nil, err
		}

		// Save to cache in background if cache is enabled
		if cacheEnabled {
			go func() {
				if _, cerr := s.cache.CacheImage(item.ID, item.ExternalID); cerr != nil {
					// already logged inside CacheImage
				}
			}()
		}

		img, _, err := image.Decode(bytes.NewReader(data))
		return img, err
	}
	return RunDBPhotoFlow(req, s.db, pick, load)
}

// loadLocalImage reads and decodes an image from a file path.
func loadLocalImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}
