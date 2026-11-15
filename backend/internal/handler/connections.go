package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
)

// ConnectionsHandler serves /api/connections: the configured instances of the
// connection-backed sources (Immich servers, Synology accounts, Google
// accounts, Unsplash and Pexels API keys), plus the per-connection actions
// that used to live under /api/immich, /api/synology, /api/google,
// /api/unsplash and /api/pexels. Secrets never leave the server:
// responses carry has_* flags, and an update keeps the stored secret when the
// field is sent empty.
type ConnectionsHandler struct {
	conns *service.ConnectionManager
}

// NewConnectionsHandler builds the handler.
func NewConnectionsHandler(conns *service.ConnectionManager) *ConnectionsHandler {
	return &ConnectionsHandler{conns: conns}
}

// connectionResponse is the redacted view of a connection.
type connectionResponse struct {
	ID        uint           `json:"id"`
	Source    string         `json:"source"`
	Name      string         `json:"name"`
	Enabled   bool           `json:"enabled"`
	Connected bool           `json:"connected"`
	Config    map[string]any `json:"config"`
	CreatedAt time.Time      `json:"created_at"`
}

// connectionRequest is the body of create and update. Config carries the
// source's fields; unknown ones are ignored.
type connectionRequest struct {
	Source  string         `json:"source"`
	Name    string         `json:"name"`
	Enabled *bool          `json:"enabled"`
	Config  map[string]any `json:"config"`
}

func (h *ConnectionsHandler) view(c model.Connection) connectionResponse {
	out := connectionResponse{
		ID: c.ID, Source: c.Source, Name: c.Name, Enabled: c.Enabled,
		Config: map[string]any{}, CreatedAt: c.CreatedAt,
	}
	switch c.Source {
	case model.SourceImmich:
		cfg := c.ImmichConfig()
		out.Connected = cfg.URL != "" && cfg.APIKey != ""
		out.Config = map[string]any{
			"url":                        cfg.URL,
			"has_api_key":                cfg.APIKey != "",
			"memory_mode":                cfg.MemoryMode,
			"auto_sync_enabled":          cfg.AutoSyncEnabled,
			"auto_sync_interval_minutes": cfg.AutoSyncIntervalMinutes,
		}
	case model.SourceSynologyPhotos:
		cfg := c.SynologyConfig()
		out.Connected = c.SynologyState().SID != ""
		out.Config = map[string]any{
			"url":                        cfg.URL,
			"account":                    cfg.Account,
			"has_password":               cfg.Password != "",
			"skip_cert":                  cfg.SkipCert,
			"auto_sync_enabled":          cfg.AutoSyncEnabled,
			"auto_sync_interval_minutes": cfg.AutoSyncIntervalMinutes,
		}
	case model.SourceGooglePhotos:
		st := c.GoogleState()
		out.Connected = st.Connected()
		// The account email is display state, not a secret.
		out.Config = map[string]any{"email": st.Email}
	case model.SourceUnsplash, model.SourcePexels:
		cfg := c.TopicConfig()
		out.Connected = cfg.APIKey != ""
		out.Config = map[string]any{
			"has_api_key":                cfg.APIKey != "",
			"randomize_results":          cfg.RandomizeResults,
			"auto_sync_enabled":          cfg.AutoSyncEnabled,
			"auto_sync_interval_minutes": cfg.AutoSyncIntervalMinutes,
		}
	}
	return out
}

func str(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return strings.TrimSpace(s), ok
}

// rawStr is str without trimming, for secrets whose spaces are significant.
func rawStr(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func boolean(m map[string]any, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		return strings.EqualFold(b, "true"), true
	}
	return false, false
}

func integer(m map[string]any, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		return i, err == nil
	}
	return 0, false
}

// applyConfig merges the request's config fields into the stored config.
// Secrets (api_key, password) are replaced only when sent non-empty.
func applyConfig(c *model.Connection, in map[string]any) error {
	if in == nil {
		in = map[string]any{}
	}
	switch c.Source {
	case model.SourceImmich:
		cfg := c.ImmichConfig()
		if v, ok := str(in, "url"); ok {
			cfg.URL = strings.TrimRight(v, "/")
		}
		if v, ok := str(in, "api_key"); ok && v != "" {
			cfg.APIKey = v
		}
		if v, ok := str(in, "memory_mode"); ok {
			if v != service.ImmichMemoryModeLatest {
				v = service.ImmichMemoryModeAll
			}
			cfg.MemoryMode = v
		}
		if v, ok := boolean(in, "auto_sync_enabled"); ok {
			cfg.AutoSyncEnabled = v
		}
		if v, ok := integer(in, "auto_sync_interval_minutes"); ok && v > 0 {
			cfg.AutoSyncIntervalMinutes = v
		}
		if cfg.AutoSyncIntervalMinutes <= 0 {
			cfg.AutoSyncIntervalMinutes = 60
		}
		return c.EncodeConfig(cfg)
	case model.SourceSynologyPhotos:
		cfg := c.SynologyConfig()
		if v, ok := str(in, "url"); ok {
			cfg.URL = strings.TrimRight(v, "/")
		}
		if v, ok := str(in, "account"); ok {
			cfg.Account = v
		}
		if v, ok := rawStr(in, "password"); ok && v != "" {
			cfg.Password = v
		}
		if v, ok := boolean(in, "skip_cert"); ok {
			cfg.SkipCert = v
		}
		if v, ok := boolean(in, "auto_sync_enabled"); ok {
			cfg.AutoSyncEnabled = v
		}
		if v, ok := integer(in, "auto_sync_interval_minutes"); ok && v > 0 {
			cfg.AutoSyncIntervalMinutes = v
		}
		if cfg.AutoSyncIntervalMinutes <= 0 {
			cfg.AutoSyncIntervalMinutes = 60
		}
		return c.EncodeConfig(cfg)
	case model.SourceGooglePhotos:
		// Nothing to configure: the account is chosen on Google's consent
		// page and the OAuth app credentials are global settings.
		c.Config = "{}"
		return nil
	case model.SourceUnsplash, model.SourcePexels:
		cfg := c.TopicConfig()
		if v, ok := str(in, "api_key"); ok && v != "" {
			cfg.APIKey = v
		}
		if v, ok := boolean(in, "randomize_results"); ok {
			cfg.RandomizeResults = v
		}
		if v, ok := boolean(in, "auto_sync_enabled"); ok {
			cfg.AutoSyncEnabled = v
		}
		if v, ok := integer(in, "auto_sync_interval_minutes"); ok && v > 0 {
			cfg.AutoSyncIntervalMinutes = v
		}
		if cfg.AutoSyncIntervalMinutes <= 0 {
			cfg.AutoSyncIntervalMinutes = 60
		}
		return c.EncodeConfig(cfg)
	}
	return errors.New("unsupported source")
}

// synologyCredentialsChanged reports whether an update touched the fields a
// DSM session is bound to.
func synologyCredentialsChanged(before, after model.Connection) bool {
	a, b := before.SynologyConfig(), after.SynologyConfig()
	return a.URL != b.URL || a.Account != b.Account || a.Password != b.Password
}

func defaultName(source string) string {
	switch source {
	case model.SourceImmich:
		return "Immich"
	case model.SourceSynologyPhotos:
		return "Synology Photos"
	case model.SourceGooglePhotos:
		return "Google Photos"
	case model.SourceUnsplash:
		return "Unsplash"
	case model.SourcePexels:
		return "Pexels"
	}
	return source
}

func (h *ConnectionsHandler) param(c echo.Context) (uint, error) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		return 0, errors.New("invalid connection id")
	}
	return uint(id), nil
}

func (h *ConnectionsHandler) load(c echo.Context) (model.Connection, bool) {
	id, err := h.param(c)
	if err != nil {
		_ = respondError(c, http.StatusBadRequest, err.Error())
		return model.Connection{}, false
	}
	conn, err := h.conns.Store().Get(id)
	if err != nil {
		if errors.Is(err, service.ErrConnectionNotFound) {
			_ = respondError(c, http.StatusNotFound, "connection not found")
		} else {
			_ = respondError(c, http.StatusInternalServerError, err.Error())
		}
		return model.Connection{}, false
	}
	return conn, true
}

// List returns every connection, optionally filtered by ?source=.
// GET /api/connections
func (h *ConnectionsHandler) List(c echo.Context) error {
	rows, err := h.conns.Store().List(c.QueryParam("source"))
	if err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	out := make([]connectionResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, h.view(r))
	}
	return c.JSON(http.StatusOK, out)
}

// Get returns one connection.
// GET /api/connections/:id
func (h *ConnectionsHandler) Get(c echo.Context) error {
	conn, ok := h.load(c)
	if !ok {
		return nil
	}
	return c.JSON(http.StatusOK, h.view(conn))
}

// Create adds a connection and starts its service.
// POST /api/connections  body: {source, name, enabled, config}
func (h *ConnectionsHandler) Create(c echo.Context) error {
	var req connectionRequest
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}
	if !model.IsConnectionSource(req.Source) {
		return respondError(c, http.StatusBadRequest, "source must be one of: "+strings.Join(model.ConnectionSources, ", "))
	}
	conn := model.Connection{Source: req.Source, Name: strings.TrimSpace(req.Name), Enabled: true}
	if conn.Name == "" {
		conn.Name = defaultName(req.Source)
	}
	if req.Enabled != nil {
		conn.Enabled = *req.Enabled
	}
	if err := applyConfig(&conn, req.Config); err != nil {
		return respondError(c, http.StatusBadRequest, err.Error())
	}
	if err := h.conns.Create(&conn); err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, h.view(conn))
}

// Update changes name, enabled and config fields; omitted fields keep their
// value, and an empty secret keeps the stored one.
// PUT /api/connections/:id
func (h *ConnectionsHandler) Update(c echo.Context) error {
	conn, ok := h.load(c)
	if !ok {
		return nil
	}
	var req connectionRequest
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		conn.Name = name
	}
	if req.Enabled != nil {
		conn.Enabled = *req.Enabled
	}
	before := conn
	if req.Config != nil {
		if err := applyConfig(&conn, req.Config); err != nil {
			return respondError(c, http.StatusBadRequest, err.Error())
		}
	}
	if err := h.conns.Store().Update(&conn); err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	// A Synology session belongs to the account it was opened for: new
	// credentials (another NAS, another account, a new password) must log
	// in afresh rather than resume the old session. A rename or a sync
	// option keeps it.
	if conn.Source == model.SourceSynologyPhotos && synologyCredentialsChanged(before, conn) {
		if err := h.conns.Store().SetState(conn.ID, "{}"); err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
	}
	h.conns.Updated(conn.ID)
	fresh, err := h.conns.Store().Get(conn.ID)
	if err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, h.view(fresh))
}

// Delete removes a connection with its albums and photos.
// DELETE /api/connections/:id
func (h *ConnectionsHandler) Delete(c echo.Context) error {
	id, err := h.param(c)
	if err != nil {
		return respondError(c, http.StatusBadRequest, err.Error())
	}
	if err := h.conns.Delete(id); err != nil {
		if errors.Is(err, service.ErrConnectionNotFound) {
			return respondError(c, http.StatusNotFound, "connection not found")
		}
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}

// Test verifies the connection's credentials against the server. For Synology
// it logs in, taking an optional one-time code for accounts with 2FA; for
// Unsplash and Pexels it runs a one-photo search with the API key.
// POST /api/connections/:id/test  body: {otp_code}
func (h *ConnectionsHandler) Test(c echo.Context) error {
	conn, ok := h.load(c)
	if !ok {
		return nil
	}
	var req struct {
		OTPCode string `json:"otp_code"`
	}
	_ = c.Bind(&req) // no body is fine
	switch conn.Source {
	case model.SourceImmich:
		svc, err := h.conns.Immich(conn.ID)
		if err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
		if err := svc.TestConnection(); err != nil {
			return respondError(c, http.StatusBadRequest, err.Error())
		}
	case model.SourceSynologyPhotos:
		svc, err := h.conns.Synology(conn.ID)
		if err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
		if err := svc.TestConnection(req.OTPCode); err != nil {
			return respondError(c, http.StatusBadRequest, err.Error())
		}
	case model.SourceGooglePhotos:
		return respondError(c, http.StatusBadRequest, "Google connections are authorised through /google/login")
	case model.SourceUnsplash, model.SourcePexels:
		svc, err := h.conns.Topic(conn.ID)
		if err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
		if err := svc.TestConnection(); err != nil {
			return respondError(c, http.StatusBadRequest, err.Error())
		}
	}
	fresh, _ := h.conns.Store().Get(conn.ID)
	return c.JSON(http.StatusOK, h.view(fresh))
}

// RemoteAlbums lists the albums the server itself offers, in the source's own
// shape (Immich: id/albumName; Synology: id/name/shared_with_me).
// GET /api/connections/:id/remote-albums
func (h *ConnectionsHandler) RemoteAlbums(c echo.Context) error {
	conn, ok := h.load(c)
	if !ok {
		return nil
	}
	switch conn.Source {
	case model.SourceImmich:
		svc, err := h.conns.Immich(conn.ID)
		if err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
		albums, err := svc.ListAlbums()
		if err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, albums)
	case model.SourceSynologyPhotos:
		svc, err := h.conns.Synology(conn.ID)
		if err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
		albums, err := svc.ListAlbums()
		if err != nil {
			if strings.Contains(err.Error(), "authentication expired") || strings.Contains(err.Error(), "not configured") {
				return respondError(c, http.StatusUnauthorized, "Session expired. Please reconnect to Synology.")
			}
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, albums)
	}
	return respondError(c, http.StatusBadRequest, "unsupported source")
}

// SetSyncAlbums defines which of the connection's albums to sync. Immich also
// takes the virtual collections.
// POST /api/connections/:id/sync-albums  body: {album_ids, favorites, all, memories}
func (h *ConnectionsHandler) SetSyncAlbums(c echo.Context) error {
	conn, ok := h.load(c)
	if !ok {
		return nil
	}
	var req struct {
		AlbumIDs  []string `json:"album_ids"`
		Favorites bool     `json:"favorites"`
		All       bool     `json:"all"`
		Memories  bool     `json:"memories"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}
	switch conn.Source {
	case model.SourceImmich:
		svc, err := h.conns.Immich(conn.ID)
		if err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
		if err := svc.SetSyncAlbums(req.AlbumIDs, req.Favorites, req.All, req.Memories); err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
	case model.SourceSynologyPhotos:
		svc, err := h.conns.Synology(conn.ID)
		if err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
		if err := svc.SetSyncAlbums(req.AlbumIDs); err != nil {
			if strings.Contains(err.Error(), "authentication expired") {
				return respondError(c, http.StatusUnauthorized, "Session expired. Please reconnect to Synology.")
			}
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Logout drops a Synology session or forgets a Google account's token. The
// connection, its album selection and (for Google) its imported photos stay.
// POST /api/connections/:id/logout
func (h *ConnectionsHandler) Logout(c echo.Context) error {
	conn, ok := h.load(c)
	if !ok {
		return nil
	}
	switch conn.Source {
	case model.SourceSynologyPhotos:
		svc, err := h.conns.Synology(conn.ID)
		if err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
		if err := svc.Logout(); err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
	case model.SourceGooglePhotos:
		svc, err := h.conns.Google(conn.ID)
		if err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
		if err := svc.Logout(); err != nil {
			return respondError(c, http.StatusInternalServerError, err.Error())
		}
	default:
		return respondError(c, http.StatusBadRequest, "only Synology and Google connections have a session to log out of")
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// topic resolves an Unsplash or Pexels connection's service or writes the
// error response.
func (h *ConnectionsHandler) topic(c echo.Context) (*service.TopicSource, bool) {
	conn, ok := h.load(c)
	if !ok {
		return nil, false
	}
	if !model.IsTopicSource(conn.Source) {
		_ = respondError(c, http.StatusBadRequest, "not an Unsplash or Pexels connection")
		return nil, false
	}
	svc, err := h.conns.Topic(conn.ID)
	if err != nil {
		_ = respondError(c, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	return svc, true
}

// Topics lists the connection's search topics (its albums, by name).
// GET /api/connections/:id/topics
func (h *ConnectionsHandler) Topics(c echo.Context) error {
	svc, ok := h.topic(c)
	if !ok {
		return nil
	}
	topics, err := svc.Topics()
	if err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string][]string{"topics": topics})
}

// SetTopics replaces the connection's topic set. Each topic becomes a synced
// album; a removed topic's album and photos go. Nothing is imported until the
// next sync.
// POST /api/connections/:id/topics  body: {topics}
func (h *ConnectionsHandler) SetTopics(c echo.Context) error {
	svc, ok := h.topic(c)
	if !ok {
		return nil
	}
	var req struct {
		Topics []string `json:"topics"`
	}
	if err := c.Bind(&req); err != nil {
		return respondError(c, http.StatusBadRequest, "invalid request")
	}
	if err := svc.SetSyncTopics(req.Topics); err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	h.conns.Updated(svc.ConnectionID())
	topics, err := svc.Topics()
	if err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string][]string{"topics": topics})
}

// google resolves a Google connection's service or writes the error response.
func (h *ConnectionsHandler) google(c echo.Context) (*service.GooglePhotosService, bool) {
	conn, ok := h.load(c)
	if !ok {
		return nil, false
	}
	if conn.Source != model.SourceGooglePhotos {
		_ = respondError(c, http.StatusBadRequest, "not a Google Photos connection")
		return nil, false
	}
	svc, err := h.conns.Google(conn.ID)
	if err != nil {
		_ = respondError(c, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	return svc, true
}

// GoogleLogin starts the OAuth consent flow for a Google connection and
// returns the URL to send the browser to. The callback lands on
// /api/auth/google/callback and redirects back to the dashboard.
// GET /api/connections/:id/google/login
func (h *ConnectionsHandler) GoogleLogin(c echo.Context) error {
	conn, ok := h.load(c)
	if !ok {
		return nil
	}
	if conn.Source != model.SourceGooglePhotos {
		return respondError(c, http.StatusBadRequest, "not a Google Photos connection")
	}
	url, err := h.conns.BeginGoogleAuth(conn.ID, buildRedirectURL(c))
	if err != nil {
		return respondError(c, http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"url": url})
}

// PickerSession opens a Google Photos Picker session for the account.
// GET /api/connections/:id/google/picker/session
func (h *ConnectionsHandler) PickerSession(c echo.Context) error {
	svc, ok := h.google(c)
	if !ok {
		return nil
	}
	id, uri, err := svc.CreatePickerSession()
	if err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"id": id, "pickerUri": uri})
}

// PickerPoll reports whether the user has finished picking.
// GET /api/connections/:id/google/picker/poll/:session
func (h *ConnectionsHandler) PickerPoll(c echo.Context) error {
	svc, ok := h.google(c)
	if !ok {
		return nil
	}
	session := c.Param("session")
	if session == "" {
		return respondError(c, http.StatusBadRequest, "session id required")
	}
	complete, err := svc.PollPickerSession(session)
	if err != nil {
		return respondError(c, http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]bool{"complete": complete})
}

// PickerProcess starts downloading the picked items into the connection in the
// background; PickerProgress reports how far it got.
// POST /api/connections/:id/google/picker/process/:session
func (h *ConnectionsHandler) PickerProcess(c echo.Context) error {
	svc, ok := h.google(c)
	if !ok {
		return nil
	}
	session := c.Param("session")
	if session == "" {
		return respondError(c, http.StatusBadRequest, "session id required")
	}
	go func() {
		// Failures are recorded in the session's progress state.
		_, _ = svc.ProcessPickerSession(session)
	}()
	return c.JSON(http.StatusAccepted, map[string]string{"status": "processing"})
}

// PickerProgress returns the import progress of a picker session.
// GET /api/connections/:id/google/picker/progress/:session
func (h *ConnectionsHandler) PickerProgress(c echo.Context) error {
	svc, ok := h.google(c)
	if !ok {
		return nil
	}
	progress := svc.PickerProgress(c.Param("session"))
	if progress == nil {
		return respondError(c, http.StatusNotFound, "session not found")
	}
	return c.JSON(http.StatusOK, progress)
}

// backend resolves the generic sync surface or writes the error response.
func (h *ConnectionsHandler) backend(c echo.Context) (service.SyncBackend, bool) {
	id, err := h.param(c)
	if err != nil {
		_ = respondError(c, http.StatusBadRequest, err.Error())
		return nil, false
	}
	b, err := h.conns.Backend(id)
	if err != nil {
		if errors.Is(err, service.ErrConnectionNotFound) {
			_ = respondError(c, http.StatusNotFound, "connection not found")
		} else {
			_ = respondError(c, http.StatusInternalServerError, err.Error())
		}
		return nil, false
	}
	return b, true
}

// Sync starts a resync in the background.
// POST /api/connections/:id/sync
func (h *ConnectionsHandler) Sync(c echo.Context) error {
	b, ok := h.backend(c)
	if !ok {
		return nil
	}
	return NewPhotoSyncHandler(b).Sync(c)
}

// SyncStatus reports whether a sync is running and the last failure.
// GET /api/connections/:id/sync-status
func (h *ConnectionsHandler) SyncStatus(c echo.Context) error {
	b, ok := h.backend(c)
	if !ok {
		return nil
	}
	return NewPhotoSyncHandler(b).SyncStatus(c)
}

// Clear removes the connection's photos.
// POST /api/connections/:id/clear
func (h *ConnectionsHandler) Clear(c echo.Context) error {
	b, ok := h.backend(c)
	if !ok {
		return nil
	}
	return NewPhotoSyncHandler(b).Clear(c)
}

// Count returns the connection's photo count.
// GET /api/connections/:id/count
func (h *ConnectionsHandler) Count(c echo.Context) error {
	b, ok := h.backend(c)
	if !ok {
		return nil
	}
	return NewPhotoSyncHandler(b).Count(c)
}
