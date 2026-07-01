package service

import (
	"strconv"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/artic"
)

// NewArticService constructs the Art Institute of Chicago topic source. ARTIC
// needs no API key, so it is always configured.
func NewArticService(db *gorm.DB, settings *SettingsService) *topicSource {
	client := artic.New()
	search := func(query string, limit int) ([]RemoteAsset, error) {
		artworks, err := client.Search(query, limit)
		if err != nil {
			return nil, err
		}
		assets := make([]RemoteAsset, 0, len(artworks))
		for _, a := range artworks {
			assets = append(assets, RemoteAsset{
				ExternalID: strconv.Itoa(a.ID),
				FilePath:   a.ImageURL(),
				Caption:    a.Title + " — " + a.ArtistTitle,
				Width:      a.Thumbnail.Width,
				Height:     a.Thumbnail.Height,
			})
		}
		return assets, nil
	}
	return newTopicSource(db, settings, model.SourceArtic, "", false, search)
}
