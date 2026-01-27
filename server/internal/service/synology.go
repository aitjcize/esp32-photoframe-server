package service

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aitjcize/photoframe-server/server/internal/model"
	"github.com/aitjcize/photoframe-server/server/pkg/synology"
	"gorm.io/gorm"
)

type SynologyService struct {
	db       *gorm.DB
	settings *SettingsService
	client   *synology.Client
	mu       sync.Mutex
}

func NewSynologyService(db *gorm.DB, settings *SettingsService) *SynologyService {
	return &SynologyService{
		db:       db,
		settings: settings,
	}
}

// ensureClient initializes and logs in the client if needed
func (s *SynologyService) ensureClient(otpCode string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	baseURL, _ := s.settings.Get("synology_url")
	account, _ := s.settings.Get("synology_account")
	password, _ := s.settings.Get("synology_password")
	savedSID, _ := s.settings.Get("synology_sid")
	savedDID, _ := s.settings.Get("synology_did")
	savedToken, _ := s.settings.Get("synology_token")
	skipCertStr, _ := s.settings.Get("synology_skip_cert")
	insecure := skipCertStr == "true"

	if baseURL == "" || account == "" || password == "" {
		return errors.New("synology credentials not configured")
	}

	var parsedURL *url.URL

	// If client exists and has same config, check connectivity or session
	if s.client == nil || s.client.BaseURL != baseURL || s.client.Account != account {
		c, err := synology.NewClient(baseURL, account, password, insecure)
		if err != nil {
			return err
		}
		s.client = c
		// Restore SID, DID and SynoToken if available
		if savedSID != "" {
			s.client.SID = savedSID
			s.client.DID = savedDID
			s.client.SynoToken = savedToken

			// Set cookies in the jar
			parsedURL, err = url.Parse(baseURL)
			if err == nil && s.client.Jar() != nil {
				cookies := []*http.Cookie{
					{Name: "id", Value: savedSID, Path: "/"},
				}
				if savedDID != "" {
					cookies = append(cookies, &http.Cookie{Name: "did", Value: savedDID, Path: "/"})
				}
				s.client.Jar().SetCookies(parsedURL, cookies)
			}
		}
	}

	// Login if no SID or if explicitly requested (otpCode provided implies re-login attempt?)
	if s.client.SID == "" || otpCode != "" {
		if err := s.client.Login(otpCode); err != nil {
			return err
		}
		// Save SID and DID
		s.settings.Set("synology_sid", s.client.SID)
		s.settings.Set("synology_did", s.client.DID)
	}

	// Capture SynoToken from cookie jar or direct client field
	_ = s.settings.Set("synology_token", s.client.SynoToken)

	return nil
}

func (s *SynologyService) Logout() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		_ = s.client.Logout()
	}
	s.settings.Set("synology_token", "")
	s.settings.Set("synology_did", "")
	return s.settings.Set("synology_sid", "")
}

func (s *SynologyService) TestConnection(otpCode string) error {
	// Force login to test
	s.mu.Lock()
	// reset client to force reload settings (but ensureClient handles it)
	s.client = nil
	s.mu.Unlock()

	return s.ensureClient(otpCode)
}

func (s *SynologyService) GetPhoto(id int, cacheKeyStr, size string) ([]byte, error) {
	if err := s.ensureClient(""); err != nil {
		return nil, err
	}

	// 1. Find photo in DB to get space and stored cache key
	var img model.Image
	if err := s.db.Where("synology_photo_id = ? AND source = ?", id, "synology").First(&img).Error; err != nil {
		// Fallback if not found in DB
		return s.client.GetPhoto(id, cacheKeyStr, size, "personal", 0, s.client.SynoToken)
	}

	// 2. Get albumID from settings for the request
	albumIDStr, _ := s.settings.Get("synology_album_id")
	var albumID int
	if albumIDStr != "" {
		id, err := strconv.Atoi(albumIDStr)
		if err == nil {
			albumID = id
		}
	}

	// Use stored ThumbnailKey (cache_key), SynologySpace, albumID, and SynoToken
	return s.client.GetPhoto(id, img.ThumbnailKey, size, img.SynologySpace, albumID, s.client.SynoToken)
}

func (s *SynologyService) ListAlbums() ([]synology.Album, error) {
	if err := s.ensureClient(""); err != nil {
		return nil, err
	}

	albums, err := s.client.ListAlbums(0, 100)
	if err != nil {
		// Check if it's an auth error
		if strings.Contains(err.Error(), "code: 119") {
			s.mu.Lock()
			s.client.SID = ""
			s.mu.Unlock()
			s.settings.Set("synology_sid", "")
			return nil, errors.New("authentication expired: please reconnect")
		}
		return nil, err
	}

	// Cache the albums list
	albumsJSON, _ := json.Marshal(albums)
	s.settings.Set("synology_albums_cache", string(albumsJSON))

	return albums, nil
}

// ImportPhotos fetches photos from Synology and adds them to DB
func (s *SynologyService) ImportPhotos() error {
	if err := s.ensureClient(""); err != nil {
		return err
	}

	// Fetch photos with pagination
	offset := 0
	limit := 500 // Fetch 500 at a time
	totalFetched := 0

	// Get album ID from settings
	albumIDStr, _ := s.settings.Get("synology_album_id")
	var albumID int
	if albumIDStr != "" {
		id, err := strconv.Atoi(albumIDStr)
		if err == nil {
			albumID = id
		}
	}

	// Get space from settings
	space, _ := s.settings.Get("synology_space")
	if space == "" {
		space = "personal" // default
	}

	log.Printf("Synology ImportPhotos: albumID=%d, space=%s, limit=%d", albumID, space, limit)

	// Fetch all photos (up to 1000 total for now)
	for offset < 1000 {
		photos, err := s.client.ListPhotos(offset, limit, albumID, space)
		if err != nil {
			log.Printf("Synology ListPhotos error: %v", err)
			// Check if it's an auth error (code 119 = session expired)
			if strings.Contains(err.Error(), "code: 119") {
				// Clear the SID
				s.mu.Lock()
				s.client.SID = ""
				s.mu.Unlock()
				s.settings.Set("synology_sid", "")
				return errors.New("authentication expired: please reconnect")
			}
			return err
		}

		if len(photos) == 0 {
			break // No more photos
		}

		totalFetched += len(photos)
		log.Printf("Fetched %d photos (offset=%d, total so far=%d)", len(photos), offset, totalFetched)

		count := 0
		for _, p := range photos {
			// Dedup by SynologyID
			var existing model.Image
			result := s.db.Where("synology_photo_id = ? AND source = ?", p.ID, "synology").First(&existing)

			if result.Error == nil {
				// Update cache key if changed
				if existing.ThumbnailKey != p.Additional.Thumbnail.M {
					existing.ThumbnailKey = p.Additional.Thumbnail.M
					s.db.Save(&existing)
				}
				continue
			}

			// Create
			img := model.Image{
				SynologyPhotoID: p.ID,
				SynologySpace:   space,
				Source:          "synology",
				FilePath:        p.Filename,
				ThumbnailKey:    p.Additional.Thumbnail.M,
				CreatedAt:       time.Now(),
				Status:          "pending",
			}

			// Use XL cache key if available
			if p.Additional.Thumbnail.XL != "" {
				img.ThumbnailKey = p.Additional.Thumbnail.XL
			}

			if err := s.db.Create(&img).Error; err != nil {
				log.Printf("Failed to insert synology photo %d: %v", p.ID, err)
				continue
			}
			count++
		}

		log.Printf("Imported %d new photos from this batch", count)

		if len(photos) < limit {
			break // Last page
		}

		offset += limit
	}

	log.Printf("ImportPhotos complete: fetched %d photos total", totalFetched)
	return nil
}

// ClearPhotos deletes all Synology photos from database
func (s *SynologyService) ClearPhotos() error {
	if err := s.db.Unscoped().Where("source = ?", "synology").Delete(&model.Image{}).Error; err != nil {
		return err
	}
	log.Println("Cleared all Synology photos from database")
	return nil
}

// ClearAndResync deletes all Synology photos and re-imports them
func (s *SynologyService) ClearAndResync() error {
	if err := s.ClearPhotos(); err != nil {
		return err
	}
	return s.ImportPhotos()
}

// GetPhotoCount returns the number of Synology photos in the database
func (s *SynologyService) GetPhotoCount() (int64, error) {
	var count int64
	if err := s.db.Model(&model.Image{}).Where("source = ?", "synology").Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
