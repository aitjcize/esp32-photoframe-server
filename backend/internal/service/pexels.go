package service

import (
	"errors"
	"strconv"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/pexels"
)

const pexelsAPIKeyName = "pexels_api_key"

// NewPexelsService constructs the Pexels topic source. It reads the API key
// from settings at sync time and paginates search results until it has `limit`
// photos or the results run out. When pexels_randomize_results is enabled,
// pages are sampled randomly so each sync yields a different set.
func NewPexelsService(db *gorm.DB, settings *SettingsService) *topicSource {
	search := func(query string, limit int) ([]RemoteAsset, error) {
		key, _ := settings.Get(pexelsAPIKeyName)
		if key == "" {
			return nil, errors.New("pexels api key not configured")
		}
		client := pexels.New(key)
		randomize, _ := settings.Get("pexels_randomize_results")

		const perPage = 80 // Pexels maximum
		return searchTopicPages(perPage, limit, randomize == "true", func(page int) ([]RemoteAsset, int, error) {
			photos, total, err := client.Search(query, perPage, page)
			if err != nil {
				return nil, 0, err
			}
			assets := make([]RemoteAsset, 0, len(photos))
			for _, p := range photos {
				assets = append(assets, RemoteAsset{
					ExternalID: strconv.Itoa(p.ID),
					FilePath:   p.Src.Large2x,
					Caption:    "Photo by " + p.Photographer + " on Pexels",
					Width:      p.Width,
					Height:     p.Height,
				})
			}
			return assets, total, nil
		})
	}
	return newTopicSource(db, settings, model.SourcePexels, pexelsAPIKeyName, true, search)
}
