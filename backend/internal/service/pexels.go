package service

import (
	"strconv"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/pexels"
)

// pexelsSearch runs one topic search against Pexels with the given key. It
// paginates until it has limit photos or the results run out; with randomize
// the pages are sampled at random so each sync yields a different set.
func pexelsSearch(key string, randomize bool, query string, limit int) ([]RemoteAsset, error) {
	client := pexels.New(key)

	const perPage = 80 // Pexels maximum
	return searchTopicPages(perPage, limit, randomize, func(page int) ([]RemoteAsset, int, error) {
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
