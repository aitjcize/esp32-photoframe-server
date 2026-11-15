package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"golang.org/x/oauth2"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
)

// connectionTokenStore keeps a Google OAuth token in the state column of one
// connection row, so every Google account the user connects has its own token
// that refreshes in place.
type connectionTokenStore struct {
	conns *ConnectionStore
	id    uint
	// mu serialises read-modify-write of the state JSON: the oauth2 library
	// refreshes from whatever goroutine first needs the token.
	mu sync.Mutex
}

func (s *connectionTokenStore) state() (model.GoogleState, error) {
	c, err := s.conns.Get(s.id)
	if err != nil {
		return model.GoogleState{}, err
	}
	return c.GoogleState(), nil
}

func (s *connectionTokenStore) save(st model.GoogleState) error {
	if st == (model.GoogleState{}) {
		return s.conns.SetState(s.id, "{}")
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return s.conns.SetState(s.id, string(b))
}

func (s *connectionTokenStore) GetToken() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.state()
	if err != nil {
		return nil, err
	}
	if st.AccessToken == "" && st.RefreshToken == "" {
		return nil, errors.New("no token found")
	}
	return &oauth2.Token{
		AccessToken:  st.AccessToken,
		RefreshToken: st.RefreshToken,
		Expiry:       st.Expiry,
		TokenType:    "Bearer",
	}, nil
}

// SaveToken stores a fresh token. Google only returns the refresh token on the
// first exchange, so an empty one keeps what is stored.
func (s *connectionTokenStore) SaveToken(token *oauth2.Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.state()
	if err != nil {
		return err
	}
	st.AccessToken = token.AccessToken
	st.Expiry = token.Expiry
	if token.RefreshToken != "" {
		st.RefreshToken = token.RefreshToken
	}
	return s.save(st)
}

func (s *connectionTokenStore) ClearToken() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(model.GoogleState{})
}

// setEmail records the account email next to the token.
func (s *connectionTokenStore) setEmail(email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.state()
	if err != nil {
		return err
	}
	st.Email = email
	return s.save(st)
}

// GooglePhotosService is one connected Google account: its OAuth client and
// the picker flow that imports photos from it. Photos are downloaded to the
// data directory at import time, so serving them never needs the token; only
// authorisation and picking do.
type GooglePhotosService struct {
	db     *gorm.DB
	conns  *ConnectionStore
	id     uint
	store  *connectionTokenStore
	cfg    googlephotos.ConfigProvider
	client *googlephotos.Client
	picker *PickerService
}

// NewGooglePhotosService builds the service for connection id. cfg supplies
// the global OAuth client id and secret.
func NewGooglePhotosService(db *gorm.DB, conns *ConnectionStore, id uint, cfg googlephotos.ConfigProvider, picker *PickerService) *GooglePhotosService {
	store := &connectionTokenStore{conns: conns, id: id}
	return &GooglePhotosService{
		db:     db,
		conns:  conns,
		id:     id,
		store:  store,
		cfg:    cfg,
		client: googlephotos.NewClient(cfg, store),
		picker: picker,
	}
}

// ConnectionID is the connection this service serves.
func (s *GooglePhotosService) ConnectionID() uint { return s.id }

func (s *GooglePhotosService) scope() sourceScope {
	return sourceScope{Source: model.SourceGooglePhotos, ConnectionID: s.id}
}

// Connected reports whether the account has a usable token.
func (s *GooglePhotosService) Connected() bool {
	st, err := s.store.state()
	return err == nil && st.Connected()
}

// AuthURL returns the Google consent page for this account. redirectURL is the
// server's callback as the browser reaches it; state ties the callback to this
// connection.
func (s *GooglePhotosService) AuthURL(redirectURL, state string) (string, error) {
	cfg, err := s.cfg.GetGoogleConfig()
	if err != nil {
		return "", err
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return "", errors.New("Google API credentials are not configured; enter the client id and secret first")
	}
	s.client.SetRedirectURL(redirectURL)
	url := s.client.GetAuthURL(state)
	if url == "" {
		return "", errors.New("could not build the Google sign-in URL")
	}
	return url, nil
}

// Exchange turns the callback's code into a token for this account and records
// the account email for display. redirectURL must be the one AuthURL was given.
func (s *GooglePhotosService) Exchange(redirectURL, code string) error {
	s.client.SetRedirectURL(redirectURL)
	if err := s.client.Exchange(code); err != nil {
		return err
	}
	if email, err := s.fetchEmail(); err != nil {
		log.Printf("google connection #%d: account email lookup: %v", s.id, err)
	} else if err := s.store.setEmail(email); err != nil {
		log.Printf("google connection #%d: store email: %v", s.id, err)
	}
	return nil
}

// fetchEmail reads the connected account's address (the userinfo.email scope
// is part of the consent).
func (s *GooglePhotosService) fetchEmail() (string, error) {
	httpClient, err := s.client.GetClient()
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("userinfo: status %d: %s", resp.StatusCode, body)
	}
	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	return info.Email, nil
}

// Logout forgets the token. Imported photos stay.
func (s *GooglePhotosService) Logout() error {
	return s.client.Logout()
}

// Stop ends the account's running picker imports (and waits for them), so
// a deletion that follows removes everything they wrote.
func (s *GooglePhotosService) Stop() {
	if s.picker != nil {
		s.picker.CancelImports(s.id)
	}
}

// CreatePickerSession opens a Google Photos Picker session for this account.
func (s *GooglePhotosService) CreatePickerSession() (id, uri string, err error) {
	return s.picker.CreateSession(s.client)
}

// PollPickerSession reports whether the user has finished picking.
func (s *GooglePhotosService) PollPickerSession(sessionID string) (bool, error) {
	return s.picker.PollSession(s.client, sessionID)
}

// ProcessPickerSession downloads the picked items into this connection.
func (s *GooglePhotosService) ProcessPickerSession(sessionID string) (int, error) {
	return s.picker.ProcessSessionItems(s.client, s.id, sessionID)
}

// PickerProgress is the import progress of a session, nil if unknown.
func (s *GooglePhotosService) PickerProgress(sessionID string) *PickerProgress {
	return s.picker.GetProgress(sessionID)
}

// --- SyncBackend ---

// ClearAndResync has no meaning for Google: photos only arrive through the
// picker.
func (s *GooglePhotosService) ClearAndResync() error {
	return errors.New("Google Photos has no sync; add photos with the picker")
}

// IsSyncing is always false; the picker reports its own progress.
func (s *GooglePhotosService) IsSyncing() bool { return false }

// LastSyncError is always empty.
func (s *GooglePhotosService) LastSyncError() string { return "" }

// ClearPhotos deletes this account's imported photos: the rows (thumbnails
// are swept with them) and then the files under the data directory, in that
// order so a failure leaves orphan files rather than rows without files.
func (s *GooglePhotosService) ClearPhotos() error {
	var items []model.Image
	if err := s.scope().where(s.db.Model(&model.Image{}).Select("id", "file_path")).
		Find(&items).Error; err != nil {
		return err
	}
	if err := clearSourcePhotos(s.db, s.scope()); err != nil {
		return err
	}
	for _, item := range items {
		if item.FilePath != "" {
			os.Remove(item.FilePath)
		}
	}
	return nil
}

// GetPhotoCount counts this account's imported photos.
func (s *GooglePhotosService) GetPhotoCount() (int64, error) {
	return sourcePhotoCount(s.db, s.scope())
}
