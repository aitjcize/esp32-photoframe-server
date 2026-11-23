package model

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Setting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `json:"value"`
}

const (
	SourceGooglePhotos   = "google_photos"
	SourceGoogleCalendar = "google_calendar" // not a photo source: a Google account whose calendar frames can show
	SourceSynologyPhotos = "synology_photos"
	SourceGallery        = "gallery"
	SourceURLProxy       = "url_proxy"
	SourceAIGeneration   = "ai_generation"
	SourceImmich         = "immich"
	SourceFractal        = "fractal"
	SourceDLA            = "dla"
	// Search-topic sources: each user topic becomes a synced album.
	SourceUnsplash = "unsplash"
	SourcePexels   = "pexels"
)

type Image struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	FilePath    string `json:"file_path"`
	Caption     string `json:"caption"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Orientation string `json:"orientation"` // "landscape", "portrait"
	UserID      int64  `json:"user_id"`
	Status      string `json:"status"`                                                                            // pending, shown
	Source      string `gorm:"index:idx_images_source;index:idx_images_source_external,priority:1" json:"source"` // "local", "google_photos", "synology_photos"
	// ExternalID is the source's stable id for this asset (immich UUID, synology
	// photo id as text, unsplash/pexels id). Indexed with Source for dedup; the
	// generic replacement for the old per-source id columns (immich_asset_id,
	// synology_photo_id).
	ExternalID string `gorm:"index:idx_images_source_external,priority:2" json:"external_id"`
	// ConnectionID is the connection (one configured Immich server, one NAS
	// account, ...) this asset was synced from. Nil for sources that have no
	// connections (gallery uploads, Google Photos, topic sources). With it,
	// the same external id can exist once per connection.
	ConnectionID *uint          `gorm:"index" json:"connection_id"`
	ThumbnailKey string         `json:"thumbnail_key"`  // Cache key for Synology
	PhotoTakenAt *time.Time     `json:"photo_taken_at"` // Original photo creation/taken date
	CreatedAt    time.Time      `json:"created_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

type Device struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `json:"name"`
	Host        string `gorm:"index" json:"host"` // IP or Hostname
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Orientation string `json:"orientation"`
	BoardName   string `json:"board_name"`
	// Display color model reported by the device: "spectra6" (6-color ACeP),
	// "gc16" (16-level grayscale / IT8951), or "" (legacy, treated as spectra6).
	DisplayType   string  `json:"display_type" gorm:"default:''"`
	EnableCollage bool    `json:"enable_collage"` // Per-device collage setting
	ShowDate      bool    `json:"show_date"`
	ShowPhotoDate bool    `json:"show_photo_date"`
	ShowWeather   bool    `json:"show_weather"`
	WeatherLat    float64 `json:"weather_lat"`
	WeatherLon    float64 `json:"weather_lon"`
	AIProvider    string  `gorm:"column:ai_provider" json:"ai_provider"`
	AIModel       string  `gorm:"column:ai_model" json:"ai_model"`
	AIPrompt      string  `gorm:"column:ai_prompt" json:"ai_prompt"`
	Layout        string  `json:"layout"`       // "photo_info", "photo_overlay", "side_panel"
	DisplayMode   string  `json:"display_mode"` // "cover" or "fit"
	// Palette color name used to letterbox photos in fit mode (passed to
	// epaper-image-convert --background-color). Empty = CLI default (white).
	BackgroundColor string `json:"background_color" gorm:"default:''"`
	// Last battery level the frame reported (X-Battery-Percentage header on
	// image fetches); -1 = never reported
	BatteryLevel      int        `json:"battery_level" gorm:"default:-1"`
	BatteryReportedAt *time.Time `json:"battery_reported_at"`
	// Last firmware version the frame reported (X-Firmware-Version header
	// on image fetches); empty = never reported
	FirmwareVersion string `json:"firmware_version" gorm:"default:''"`
	// Per-device image source (e.g. "immich"), resolved by the unified /image
	// endpoint. Empty means the device hasn't picked a server-side source.
	Source string `json:"source"`
	// ImageConnectionID narrows a connection-backed Source to one connection
	// (one Immich server, one Synology account...). Nil draws from every
	// connection of the source, which is what every frame did before a
	// source could have several.
	ImageConnectionID *uint  `json:"image_connection_id"`
	ShowCalendar      bool   `json:"show_calendar"`
	CalendarID        string `json:"calendar_id"` // Google Calendar ID (per-device)
	// CalendarConnectionID is the google_calendar connection (the Google
	// account) CalendarID belongs to. Nil falls back to the first connected
	// account, which is what every frame had before accounts were plural.
	CalendarConnectionID *uint  `json:"calendar_connection_id"`
	DateFormat           string `json:"date_format"` // Go time format string, empty = default "Mon, Jan 02"
	// Password for the frame's own HTTP API when its owner enabled the
	// optional authentication. Empty = open, the firmware default. Never
	// serialized outward -- the UI posts it and only learns whether one is
	// set (see HTTPPasswordSet on the API response).
	HTTPPassword string `json:"-" gorm:"column:http_password;default:''"`
	// Reported instead of the password itself, so the UI can show whether one
	// is configured without the value ever leaving the server.
	HTTPPasswordSet bool `json:"http_password_set" gorm:"-"`
	// AuthRequired: the frame refused this server's last request for want of
	// the right password (401), so nothing the server does with the frame
	// works until the user enters the frame's password. Cleared by the next
	// request the frame answers, and when a new password is stored.
	// AuthFailedAt is when the refusals started; it stays put while they
	// last, so the webapp can tell a new refusal from the one it already
	// asked about.
	AuthRequired bool       `json:"auth_required" gorm:"default:false"`
	AuthFailedAt *time.Time `json:"auth_failed_at"`
	// Remote config sync fields (JSON blobs synced from/to device)
	DeviceConfig             string `json:"device_config" gorm:"default:'{}'"`
	DeviceProcessingSettings string `json:"device_processing_settings" gorm:"default:'{}'"`
	DeviceColorPalette       string `json:"device_color_palette" gorm:"default:'{}'"`
	ConfigLastUpdated        int64  `json:"config_last_updated" gorm:"default:0"`
	// ConfigUndelivered: the stored settings above have not reached the
	// frame yet (the direct push after an edit failed, and the frame has not
	// picked them up since). Saving them unchanged then retries the push.
	ConfigUndelivered bool      `json:"-" gorm:"default:false"`
	CreatedAt         time.Time `json:"created_at"`
}

// AfterFind reports whether a device HTTP password is stored without ever
// serializing the value. Done as a hook so every load path gets it right.
func (d *Device) AfterFind(tx *gorm.DB) error {
	d.HTTPPasswordSet = d.HTTPPassword != ""
	return nil
}

// IsGrayscale reports whether this device drives a grayscale (GC16) panel.
func (d *Device) IsGrayscale() bool {
	return strings.HasPrefix(d.DisplayType, "gc") || d.BoardName == "seeedstudio_reterminal_e1003"
}

const (
	LayoutPhotoInfo    = "photo_info"
	LayoutPhotoOverlay = "photo_overlay"
	LayoutSidePanel    = "side_panel"
)

type DeviceHistory struct {
	ID       uint      `gorm:"primaryKey" json:"id"`
	DeviceID uint      `gorm:"index" json:"device_id"` // Foreign key to Device
	ImageID  uint      `json:"image_id"`
	ServedAt time.Time `json:"served_at"`
}

type UserSession struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	TokenID   string    `gorm:"index" json:"-"`
	UserAgent string    `json:"user_agent"`
	IP        string    `json:"ip"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type URLSource struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

type DeviceURLMapping struct {
	DeviceID    uint `gorm:"primaryKey" json:"device_id"`
	URLSourceID uint `gorm:"primaryKey" json:"url_source_id"`
}

const (
	AlbumKindReal    = "album"   // a real album from the source
	AlbumKindVirtual = "virtual" // an Immich mode expressed as an album

	// Virtual album external IDs (Immich source modes as albums).
	ImmichVirtualAll       = "__all__"
	ImmichVirtualFavorites = "__favorites__"
	ImmichVirtualMemories  = "__memories__"
)

// Album is a persisted photo-source album (Immich/Synology). Real albums map
// to a source album by ExternalID; virtual albums (Immich all/favorites/
// memories) use the ImmichVirtual* sentinels. SyncEnabled marks albums the
// server pulls assets for.
type Album struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	Source     string `gorm:"index:idx_albums_source_external,priority:1" json:"source"`
	ExternalID string `gorm:"index:idx_albums_source_external,priority:2" json:"external_id"`
	// ConnectionID is the connection this album belongs to; an external id is
	// unique per connection, so two Immich servers can both have "__all__".
	// Nil for topic sources, which have no connections.
	ConnectionID *uint `gorm:"index" json:"connection_id"`
	// ConnectionName is filled in by ListAlbums for display; not stored.
	ConnectionName string `gorm:"-" json:"connection_name"`
	Name           string `json:"name"`
	Kind           string `json:"kind"` // "album" | "virtual"
	SyncEnabled    bool   `json:"sync_enabled"`
	// SharePassphrase grants access to a Synology album shared with (not owned
	// by) the logged-in account. Empty for owned albums and other sources.
	SharePassphrase string    `json:"-"`
	AssetCount      int       `gorm:"-" json:"asset_count"` // computed live in ListAlbums; not stored
	CoverKey        string    `json:"cover_key"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ImageAlbumMembership is the many-to-many link between an image (asset) and
// the albums it belongs to. One images row per asset; membership scopes it.
type ImageAlbumMembership struct {
	ImageID uint `gorm:"primaryKey" json:"image_id"`
	AlbumID uint `gorm:"primaryKey" json:"album_id"`
}

// DeviceAlbumMapping binds a device to the albums it draws photos from
// (multiple albums per device, one source per device by convention).
type DeviceAlbumMapping struct {
	DeviceID uint `gorm:"primaryKey" json:"device_id"`
	AlbumID  uint `gorm:"primaryKey" json:"album_id"`
}

// GenerativeState persists the rolling state for a procedural image source
// (fractal zoom counter, DLA occupancy grids, etc.). Keyed on (DeviceID, Source)
// so a device can switch sources without losing its progress in either.
type GenerativeState struct {
	DeviceID  uint      `gorm:"primaryKey" json:"device_id"`
	Source    string    `gorm:"primaryKey" json:"source"`
	State     []byte    `json:"-"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Connection is one configured instance of a photo provider: one Immich
// server, one Synology account, one Google account, one Unsplash or Pexels
// API key. The source string stays
// the provider type; albums and images hang off the connection, so several
// connections of the same type can coexist. Config holds the user-entered
// settings (credentials included) as JSON; State holds what the server learned
// on its own (a NAS session, an OAuth token) so a user edit never races it. Both stay out of JSON responses;
// the handler reports a redacted view.
type Connection struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Source    string    `gorm:"index" json:"source"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Config    string    `gorm:"default:'{}'" json:"-"`
	State     string    `gorm:"default:'{}'" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConnectionSources are the source types that are configured as connections.
var ConnectionSources = []string{SourceImmich, SourceSynologyPhotos, SourceGooglePhotos, SourceUnsplash, SourcePexels, SourceGoogleCalendar}

// TopicSources are the connection sources whose albums are search topics
// (stock-photo search by keyword).
var TopicSources = []string{SourceUnsplash, SourcePexels}

// IsTopicSource reports whether a source is a search-topic source.
func IsTopicSource(source string) bool {
	for _, s := range TopicSources {
		if s == source {
			return true
		}
	}
	return false
}

// IsConnectionSource reports whether a source type is configured through
// connections rather than through flat settings keys.
func IsConnectionSource(source string) bool {
	for _, s := range ConnectionSources {
		if s == source {
			return true
		}
	}
	return false
}

// ImmichConfig is Connection.Config for an Immich connection.
type ImmichConfig struct {
	URL    string `json:"url"`
	APIKey string `json:"api_key"`
	// MemoryMode selects the "memories" lanes: "all" (every year, default) or
	// "latest" (the most recent year only).
	MemoryMode              string `json:"memory_mode"`
	AutoSyncEnabled         bool   `json:"auto_sync_enabled"`
	AutoSyncIntervalMinutes int    `json:"auto_sync_interval_minutes"`
}

// SynologyConfig is Connection.Config for a Synology Photos connection.
type SynologyConfig struct {
	URL                     string `json:"url"`
	Account                 string `json:"account"`
	Password                string `json:"password"`
	SkipCert                bool   `json:"skip_cert"`
	AutoSyncEnabled         bool   `json:"auto_sync_enabled"`
	AutoSyncIntervalMinutes int    `json:"auto_sync_interval_minutes"`
}

// TopicConfig is Connection.Config for an Unsplash or Pexels connection: the
// API key and how its topics are synced.
type TopicConfig struct {
	APIKey string `json:"api_key"`
	// RandomizeResults samples random pages of each topic's results on every
	// sync, so the albums rotate instead of keeping the top results.
	RandomizeResults        bool `json:"randomize_results"`
	AutoSyncEnabled         bool `json:"auto_sync_enabled"`
	AutoSyncIntervalMinutes int  `json:"auto_sync_interval_minutes"`
}

// SynologyState is Connection.State for a Synology Photos connection: the
// DSM session the server holds, restored across restarts.
type SynologyState struct {
	SID   string `json:"sid"`
	DID   string `json:"did"`
	Token string `json:"token"`
}

// GoogleState is Connection.State for a Google Photos or Google Calendar
// connection: the OAuth
// token Google issued for that account, refreshed in place, plus the account
// email for display. A Google connection has no Config; the OAuth client id
// and secret are global settings shared by every Google connection.
type GoogleState struct {
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
	Email        string    `json:"email,omitempty"`
}

// Connected reports whether the token can still be used: valid now, or
// refreshable.
func (st GoogleState) Connected() bool {
	if st.RefreshToken != "" {
		return true
	}
	return st.AccessToken != "" && st.Expiry.After(time.Now())
}

// DecodeConfig unmarshals Config into v. An empty Config is the zero value.
func (c *Connection) DecodeConfig(v any) error {
	if strings.TrimSpace(c.Config) == "" {
		return nil
	}
	return json.Unmarshal([]byte(c.Config), v)
}

// EncodeConfig stores v as Config.
func (c *Connection) EncodeConfig(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.Config = string(b)
	return nil
}

// DecodeState unmarshals State into v. An empty State is the zero value.
func (c *Connection) DecodeState(v any) error {
	if strings.TrimSpace(c.State) == "" {
		return nil
	}
	return json.Unmarshal([]byte(c.State), v)
}

// ImmichConfig decodes Config for an Immich connection (zero value on error).
func (c *Connection) ImmichConfig() ImmichConfig {
	var cfg ImmichConfig
	_ = c.DecodeConfig(&cfg)
	return cfg
}

// SynologyConfig decodes Config for a Synology connection (zero value on error).
func (c *Connection) SynologyConfig() SynologyConfig {
	var cfg SynologyConfig
	_ = c.DecodeConfig(&cfg)
	return cfg
}

// TopicConfig decodes Config for an Unsplash or Pexels connection (zero value
// on error).
func (c *Connection) TopicConfig() TopicConfig {
	var cfg TopicConfig
	_ = c.DecodeConfig(&cfg)
	return cfg
}

// SynologyState decodes State for a Synology connection (zero value on error).
func (c *Connection) SynologyState() SynologyState {
	var st SynologyState
	_ = c.DecodeState(&st)
	return st
}

// GoogleState decodes State for a Google Photos connection (zero value on error).
func (c *Connection) GoogleState() GoogleState {
	var st GoogleState
	_ = c.DecodeState(&st)
	return st
}
