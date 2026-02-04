package googlephotos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Album struct {
	ID                    string `json:"id"`
	Title                 string `json:"title"`
	ProductUrl            string `json:"productUrl"`
	MediaItemsCount       string `json:"mediaItemsCount"`
	CoverPhotoBaseUrl     string `json:"coverPhotoBaseUrl"`
	CoverPhotoMediaItemId string `json:"coverPhotoMediaItemId"`
}

type MediaItem struct {
	ID         string `json:"id"`
	ProductUrl string `json:"productUrl"`
	BaseUrl    string `json:"baseUrl"`
	MimeType   string `json:"mimeType"`
	Filename   string `json:"filename"`
}

type albumsResponse struct {
	Albums        []Album `json:"albums"`
	NextPageToken string  `json:"nextPageToken"`
}

type mediaItemsResponse struct {
	MediaItems    []MediaItem `json:"mediaItems"`
	NextPageToken string      `json:"nextPageToken"`
}

func (c *Client) ListAlbums(pageSize int) ([]Album, error) {
	client, err := c.GetClient()
	if err != nil {
		return nil, err
	}

	endpoint := "https://photoslibrary.googleapis.com/v1/albums"
	u, _ := url.Parse(endpoint)
	q := u.Query()
	q.Set("pageSize", fmt.Sprintf("%d", pageSize))
	u.RawQuery = q.Encode()

	resp, err := client.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		return nil, fmt.Errorf("google photos api returned status: %d, body: %s", resp.StatusCode, buf.String())
	}

	var result albumsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Albums, nil
}

func (c *Client) ListMediaItems(albumID string, pageSize int) ([]MediaItem, error) {
	client, err := c.GetClient()
	if err != nil {
		return nil, err
	}

	endpoint := "https://photoslibrary.googleapis.com/v1/mediaItems:search"

	body := map[string]interface{}{
		"albumId":  albumID,
		"pageSize": pageSize,
	}

	jsonBody, _ := json.Marshal(body)

	resp, err := client.Post(endpoint, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google photos api returned status: %d", resp.StatusCode)
	}

	var result mediaItemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.MediaItems, nil
}
