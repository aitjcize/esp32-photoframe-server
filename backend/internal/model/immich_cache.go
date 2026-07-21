package model

import "time"

// ImmichCache holds a locally-cached copy of an Immich asset's image bytes so
// the photoframe can keep serving photos when the Immich server is unreachable.
type ImmichCache struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ImageID   uint      `gorm:"index" json:"image_id"`
	AssetID   string    `gorm:"uniqueIndex" json:"asset_id"`
	FilePath  string    `json:"file_path"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	SizeBytes int64     `json:"size_bytes"`
	CachedAt  time.Time `json:"cached_at"`
}
