package service

import (
	"errors"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/unsplash"
)

const unsplashAPIKeyName = "unsplash_api_key"

// NewUnsplashService constructs the Unsplash topic source. It reads the API key
// from settings at sync time and paginates search results until it has `limit`
// photos or the results run out.
func NewUnsplashService(db *gorm.DB, settings *SettingsService) *topicSource {
	search := func(query string, limit int) ([]RemoteAsset, error) {
		key, _ := settings.Get(unsplashAPIKeyName)
		if key == "" {
			return nil, errors.New("unsplash api key not configured")
		}
		client := unsplash.New(key)

		const perPage = 30 // Unsplash maximum
		assets := make([]RemoteAsset, 0, limit)
		for page := 1; len(assets) < limit; page++ {
			photos, err := client.Search(query, perPage, page)
			if err != nil {
				return nil, err
			}
			if len(photos) == 0 {
				break
			}
			for _, p := range photos {
				// TODO: Unsplash's API guidelines ask clients to trigger the
				// download endpoint (p.Links.DownloadLocation) when a photo is
				// used, for attribution/analytics. Skipped for now.
				assets = append(assets, RemoteAsset{
					ExternalID: p.ID,
					FilePath:   p.URLs.Raw + "&w=1920&q=80&fm=jpg&fit=max",
					Caption:    "Photo by " + p.User.Name + " on Unsplash",
					Width:      p.Width,
					Height:     p.Height,
				})
				if len(assets) >= limit {
					break
				}
			}
		}
		return assets, nil
	}
	return newTopicSource(db, settings, model.SourceUnsplash, unsplashAPIKeyName, true, search)
}
