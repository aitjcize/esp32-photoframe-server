package service

import (
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/unsplash"
)

// unsplashSearch runs one topic search against Unsplash with the given key. It
// paginates until it has limit photos or the results run out; with randomize
// the pages are sampled at random so each sync yields a different set.
func unsplashSearch(key string, randomize bool, query string, limit int) ([]RemoteAsset, error) {
	client := unsplash.New(key)

	const perPage = 30 // Unsplash maximum
	return searchTopicPages(perPage, limit, randomize, func(page int) ([]RemoteAsset, int, error) {
		photos, total, err := client.Search(query, perPage, page)
		if err != nil {
			return nil, 0, err
		}
		assets := make([]RemoteAsset, 0, len(photos))
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
		}
		return assets, total, nil
	})
}
