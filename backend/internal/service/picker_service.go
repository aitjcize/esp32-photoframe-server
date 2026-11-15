package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/imageops"
	"gorm.io/gorm"
)

type PickerSessionResponse struct {
	ID            string `json:"id"`
	PickerUri     string `json:"pickerUri"`
	MediaItemsSet bool   `json:"mediaItemsSet"`
}

type PickedMediaItem struct {
	ID        string    `json:"id"`
	MediaFile MediaFile `json:"mediaFile"`
}

type MediaFile struct {
	BaseUrl  string `json:"baseUrl"`
	MimeType string `json:"mimeType"`
	Filename string `json:"filename"`
}

type MediaItemsResponse struct {
	MediaItems    []PickedMediaItem `json:"mediaItems"`
	NextPageToken string            `json:"nextPageToken"`
}

type PickerProgress struct {
	Total     int    `json:"total"`
	Processed int    `json:"processed"`
	Status    string `json:"status"` // "processing", "done", "error"
	Error     string `json:"error,omitempty"`
}

// PickerService runs the Google Photos Picker flow: open a session, wait for
// the user to pick, download the picked items into the data directory. It is
// shared by every Google connection; each call names the account's OAuth
// client and the connection the imported photos belong to. Session ids come
// from Google and are unique across accounts, so one progress map serves all.
type PickerService struct {
	db       *gorm.DB
	dataDir  string
	mu       sync.Mutex
	progress map[string]*PickerProgress
	// Running imports per connection, so deleting a connection can stop
	// them and wait before it removes the rows and files they add to.
	imports map[uint]*importTracker
	// Items being imported right now, by connection and item id: two picker
	// sessions that overlap on an item must not download, record or clean
	// up the same file at once.
	inflight map[string]bool
}

// importTracker counts a connection's running imports and carries the stop
// request to them: ctx is cancelled, which aborts their requests and body
// reads, and they check cancelled between items.
type importTracker struct {
	wg        sync.WaitGroup
	cancelled atomic.Bool
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewPickerService(db *gorm.DB, dataDir string) *PickerService {
	return &PickerService{
		db:       db,
		dataDir:  dataDir,
		progress: make(map[string]*PickerProgress),
		imports:  make(map[uint]*importTracker),
		inflight: make(map[string]bool),
	}
}

// claimItem marks an item of a connection as being imported; false means
// another import holds it and this one skips it. releaseItem gives it back.
func (s *PickerService) claimItem(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight[key] {
		return false
	}
	s.inflight[key] = true
	return true
}

func (s *PickerService) releaseItem(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inflight, key)
}

// beginImport registers a running import of connID; endImport releases it.
func (s *PickerService) beginImport(connID uint) *importTracker {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.imports[connID]
	if !ok {
		t = &importTracker{}
		t.ctx, t.cancel = context.WithCancel(context.Background())
		s.imports[connID] = t
	}
	t.wg.Add(1)
	return t
}

func (s *PickerService) endImport(t *importTracker) { t.wg.Done() }

// CancelImports asks the running imports of connID to stop and waits until
// they have. Called before a connection's photos and row are removed, so no
// import keeps writing files and rows for a connection that is gone.
func (s *PickerService) CancelImports(connID uint) {
	s.mu.Lock()
	t, ok := s.imports[connID]
	if ok {
		t.cancelled.Store(true)
		t.cancel()
		delete(s.imports, connID)
	}
	s.mu.Unlock()
	if ok {
		t.wg.Wait()
	}
}

func (s *PickerService) GetProgress(sessionID string) *PickerProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.progress[sessionID]; ok {
		// Return a copy so callers never hold a reference to the struct that
		// ProcessSessionItems mutates concurrently.
		cp := *p
		return &cp
	}
	return nil
}

// setProgress runs fn against the per-session progress struct while holding the
// lock, creating the entry if it does not yet exist. All mutations of
// s.progress and the per-session struct fields must go through here (or be
// otherwise guarded by s.mu).
func (s *PickerService) setProgress(sessionID string, fn func(p *PickerProgress)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.progress[sessionID]
	if !ok {
		p = &PickerProgress{}
		s.progress[sessionID] = p
	}
	fn(p)
}

func (s *PickerService) CreateSession(client *googlephotos.Client) (string, string, error) {
	httpClient, err := client.GetClient()
	if err != nil {
		return "", "", err
	}

	// Create session
	body := []byte(`{}`) // Empty body
	req, err := http.NewRequest("POST", "https://photospicker.googleapis.com/v1/sessions", bytes.NewBuffer(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("failed to create session: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var session PickerSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return "", "", err
	}

	return session.ID, session.PickerUri, nil
}

func (s *PickerService) PollSession(client *googlephotos.Client, sessionID string) (bool, error) {
	httpClient, err := client.GetClient()
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest("GET", "https://photospicker.googleapis.com/v1/sessions/"+sessionID, nil)
	if err != nil {
		return false, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false, fmt.Errorf("failed to get session: status %d", resp.StatusCode)
	}

	var session PickerSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return false, err
	}

	return session.MediaItemsSet, nil
}

// ProcessSessionItems downloads the session's picked items and records them as
// images of connection connID.
func (s *PickerService) ProcessSessionItems(client *googlephotos.Client, connID uint, sessionID string) (int, error) {
	httpClient, err := client.GetClient()
	if err != nil {
		return 0, err
	}
	tracker := s.beginImport(connID)
	defer s.endImport(tracker)

	// Initialize Progress
	s.setProgress(sessionID, func(p *PickerProgress) {
		p.Status = "listing"
	})

	// setError records a terminal error state for the session.
	setError := func(errStr string) {
		s.setProgress(sessionID, func(p *PickerProgress) {
			p.Status = "error"
			p.Error = errStr
		})
	}

	// Pagination loop
	var allItems []PickedMediaItem
	pageToken := ""

	for {
		// sessionId is REQUIRED for listing picked items
		url := fmt.Sprintf("https://photospicker.googleapis.com/v1/mediaItems?pageSize=100&sessionId=%s", sessionID)
		if pageToken != "" {
			url += "&pageToken=" + pageToken
		}

		req, err := http.NewRequestWithContext(tracker.ctx, "GET", url, nil)
		if err != nil {
			setError(err.Error())
			return 0, err
		}

		// Process a single page inside a closure so the response body is closed
		// at the end of every iteration rather than stacking deferred closes
		// until the whole function returns.
		nextToken, items, err := func() (string, []PickedMediaItem, error) {
			resp, err := httpClient.Do(req)
			if err != nil {
				setError(err.Error())
				return "", nil, err
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				body, _ := io.ReadAll(resp.Body)
				errStr := fmt.Sprintf("failed to list items: %s", string(body))
				setError(errStr)
				return "", nil, fmt.Errorf("%s", errStr)
			}

			var listResp MediaItemsResponse
			if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
				setError(err.Error())
				return "", nil, err
			}

			return listResp.NextPageToken, listResp.MediaItems, nil
		}()
		if err != nil {
			return 0, err
		}

		allItems = append(allItems, items...)
		pageToken = nextToken
		if pageToken == "" {
			break
		}
	}

	// Update Total
	s.setProgress(sessionID, func(p *PickerProgress) {
		p.Total = len(allItems)
		p.Status = "downloading"
	})

	// Download items
	count := 0
	photosDir := filepath.Join(s.dataDir, "photos")
	if err := os.MkdirAll(photosDir, 0755); err != nil {
		setError(err.Error())
		return 0, err
	}

	markProcessed := func() {
		s.setProgress(sessionID, func(p *PickerProgress) {
			p.Processed++
		})
	}

	for _, item := range allItems {
		if tracker.cancelled.Load() {
			errStr := "import stopped: the connection was deleted"
			setError(errStr)
			return count, errors.New(errStr)
		}
		// Download High Quality
		if item.MediaFile.BaseUrl == "" {
			markProcessed() // Count skipped as processed? yes
			continue
		}

		// Skip videos
		if len(item.MediaFile.MimeType) >= 5 && item.MediaFile.MimeType[:5] == "video" {
			fmt.Printf("Skipping video: %s (%s)\n", item.MediaFile.Filename, item.MediaFile.MimeType)
			markProcessed()
			continue
		}

		// Process a single item inside a closure so the download response body
		// is closed at the end of every iteration instead of stacking deferred
		// closes until the whole function returns.
		if func() bool {
			itemKey := fmt.Sprintf("%d/%s", connID, item.ID)
			if !s.claimItem(itemKey) {
				fmt.Printf("Skipping %s: another import of this connection is fetching it\n", item.MediaFile.Filename)
				return false
			}
			defer s.releaseItem(itemKey)

			downloadUrl := item.MediaFile.BaseUrl + "=w1600-h1600"

			req, err := http.NewRequestWithContext(tracker.ctx, "GET", downloadUrl, nil)
			if err != nil {
				fmt.Printf("Failed to download %s: %v\n", item.MediaFile.Filename, err)
				return false
			}
			resp, err := httpClient.Do(req)
			if err != nil {
				fmt.Printf("Failed to download %s: %v\n", item.MediaFile.Filename, err)
				return false
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				fmt.Printf("Failed to download %s: status %d\n", item.MediaFile.Filename, resp.StatusCode)
				return false
			}

			// One file per (connection, item): the same account connected
			// twice, or two accounts that share a photo, each get their own
			// copy, so one connection's clear never takes the other's photo.
			// Imports from before connections kept the bare item id.
			ext := ".jpg"
			localFilename := fmt.Sprintf("%d_%s%s", connID, item.ID, ext)
			localPath := filepath.Join(photosDir, localFilename)
			legacyPath := filepath.Join(photosDir, item.ID+ext)

			// Check for duplicate in DB (this connection's rows only)
			var existing model.Image
			if err := s.db.Where("connection_id = ? AND file_path IN ?", connID, []string{localPath, legacyPath}).
				First(&existing).Error; err == nil {
				localPath = existing.FilePath
				// Record exists. Check if file exists.
				if _, err := os.Stat(localPath); err == nil {
					// Both exist. Skip.
					fmt.Printf("Skipping duplicate: %s\n", localFilename)
					return false
				}
				// File missing, delete old record so we can re-download and strictly create new one
				s.db.Unscoped().Delete(&existing)
				localPath = filepath.Join(photosDir, localFilename)
			}

			// Download to a side file and move it into place once complete,
			// so an aborted request or a cancelled import leaves no partial
			// photo behind (nothing would ever list one for removal).
			partPath := localPath + ".part"
			out, err := os.Create(partPath)
			if err != nil {
				return false
			}
			_, err = io.Copy(out, resp.Body)
			out.Close() // Close before opening for decode
			if err != nil {
				os.Remove(partPath)
				return false
			}
			if err := os.Rename(partPath, localPath); err != nil {
				os.Remove(partPath)
				return false
			}

			// Bake EXIF orientation into the pixels. Google's CDN with size
			// hints usually returns pre-rotated pixels, but the API doesn't
			// guarantee it; auto-orient is a cheap safety net. Non-fatal.
			if err := imageops.AutoOrient(localPath); err != nil {
				fmt.Printf("auto-orient failed for google photo %s: %v\n", localPath, err)
			}

			// Decode image config to get dimensions
			f, err := os.Open(localPath)
			if err != nil {
				return false
			}
			imgConfig, _, err := image.DecodeConfig(f)
			f.Close()

			width := 0
			height := 0
			orientation := "landscape"

			if err == nil {
				width = imgConfig.Width
				height = imgConfig.Height
				if height > width {
					orientation = "portrait"
				}
			}

			// Add to DB queue
			connRef := connID
			image := model.Image{
				FilePath:     localPath,
				Source:       model.SourceGooglePhotos,
				ConnectionID: &connRef,
				UserID:       1, // Default user
				Status:       "pending",
				CreatedAt:    time.Now(),
				Caption:      "From Google Photos",
				Width:        width,
				Height:       height,
				Orientation:  orientation,
			}
			// A row that cannot be written (the connection is gone) leaves no
			// file behind either: nothing would ever list it for removal.
			if err := s.db.Create(&image).Error; err != nil {
				fmt.Printf("Failed to record %s: %v\n", item.MediaFile.Filename, err)
				os.Remove(localPath)
				return false
			}
			return true
		}() {
			count++
		}

		// Update Progress
		markProcessed()
	}

	s.setProgress(sessionID, func(p *PickerProgress) {
		p.Status = "done"
	})
	return count, nil
}
