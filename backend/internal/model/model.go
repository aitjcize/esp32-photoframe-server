package model

import (
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
	ExternalID   string         `gorm:"index:idx_images_source_external,priority:2" json:"external_id"`
	ThumbnailKey string         `json:"thumbnail_key"`  // Cache key for Synology
	PhotoTakenAt *time.Time     `json:"photo_taken_at"` // Original photo creation/taken date
	CreatedAt    time.Time      `json:"created_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

type GoogleAuth struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	AccessToken  string    `json:"-"`
	RefreshToken string    `json:"-"`
	Expiry       time.Time `json:"expiry"`
}

type GoogleCalendarAuth struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	AccessToken  string    `json:"-"`
	RefreshToken string    `json:"-"`
	Expiry       time.Time `json:"expiry"`
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
	// Per-device image source (e.g. "immich"), resolved by the unified /image
	// endpoint. Empty means the device hasn't picked a server-side source.
	Source       string `json:"source"`
	ShowCalendar bool   `json:"show_calendar"`
	CalendarID   string `json:"calendar_id"` // Google Calendar ID (per-device)
	DateFormat   string `json:"date_format"` // Go time format string, empty = default "Mon, Jan 02"
	// Remote config sync fields (JSON blobs synced from/to device)
	DeviceConfig             string    `json:"device_config" gorm:"default:'{}'"`
	DeviceProcessingSettings string    `json:"device_processing_settings" gorm:"default:'{}'"`
	DeviceColorPalette       string    `json:"device_color_palette" gorm:"default:'{}'"`
	ConfigLastUpdated        int64     `json:"config_last_updated" gorm:"default:0"`
	CreatedAt                time.Time `json:"created_at"`
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
	ID          uint      `gorm:"primaryKey" json:"id"`
	Source      string    `gorm:"uniqueIndex:idx_albums_source_external,priority:1" json:"source"`
	ExternalID  string    `gorm:"uniqueIndex:idx_albums_source_external,priority:2" json:"external_id"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"` // "album" | "virtual"
	SyncEnabled bool      `json:"sync_enabled"`
	AssetCount  int       `gorm:"-" json:"asset_count"` // computed live in ListAlbums; not stored
	CoverKey    string    `json:"cover_key"`
	UpdatedAt   time.Time `json:"updated_at"`
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
