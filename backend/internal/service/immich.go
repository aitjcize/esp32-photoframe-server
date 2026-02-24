package service

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/immich"
)

// ImmichService manages interaction with an Immich server.
type ImmichService struct {
	settings *SettingsService
	client   *immich.Client
}

// NewImmichService creates a new ImmichService.
func NewImmichService(settings *SettingsService) *ImmichService {
	return &ImmichService{
		settings: settings,
		client:   immich.NewClient(),
	}
}

// getConfig fetches the current Immich configuration from settings.
func (s *ImmichService) getConfig() (serverURL, apiKey, albumID string, err error) {
	serverURL, err = s.settings.Get("immich_url")
	if err != nil || serverURL == "" {
		return "", "", "", fmt.Errorf("immich_url not configured")
	}
	apiKey, err = s.settings.Get("immich_api_key")
	if err != nil || apiKey == "" {
		return "", "", "", fmt.Errorf("immich_api_key not configured")
	}
	albumID, _ = s.settings.Get("immich_album_id")
	return serverURL, apiKey, albumID, nil
}

// TestConnection tests the connection to the Immich server.
func (s *ImmichService) TestConnection(serverURL, apiKey string) error {
	return s.client.TestConnection(serverURL, apiKey)
}

// ListAlbums returns all albums from the configured Immich server.
func (s *ImmichService) ListAlbums(serverURL, apiKey string) ([]immich.Album, error) {
	return s.client.ListAlbums(serverURL, apiKey)
}

// FetchRandomPhoto fetches a random photo from the configured Immich server/album
// and returns it as a decoded image.
func (s *ImmichService) FetchRandomPhoto() (image.Image, error) {
	serverURL, apiKey, albumID, err := s.getConfig()
	if err != nil {
		return nil, err
	}

	// Get a random asset
	asset, err := s.client.GetRandomAsset(serverURL, apiKey, albumID)
	if err != nil {
		return nil, fmt.Errorf("failed to get random asset: %w", err)
	}

	log.Printf("Immich: fetching asset %s (%s)", asset.ID, asset.OriginalFileName)

	// Download the original file
	data, err := s.client.DownloadAsset(serverURL, apiKey, asset.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to download asset: %w", err)
	}

	// Decode the image
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image from Immich asset %s: %w", asset.ID, err)
	}

	return img, nil
}
