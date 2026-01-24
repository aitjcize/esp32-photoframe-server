package model

import (
	"time"

	"gorm.io/gorm"
)

type Setting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `json:"value"`
}

type Image struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	FilePath        string         `json:"file_path"`
	Caption         string         `json:"caption"`
	Width           int            `json:"width"`
	Height          int            `json:"height"`
	Orientation     string         `json:"orientation"` // "landscape", "portrait"
	UserID          int64          `json:"user_id"`
	Status          string         `json:"status"` // pending, shown
	Source          string         `json:"source"` // "local", "google", "synology"
	SynologyPhotoID int            `json:"synology_id"`
	SynologySpace   string         `json:"synology_space"` // "personal" or "shared"
	ThumbnailKey    string         `json:"thumbnail_key"`  // Cache key for Synology
	CreatedAt       time.Time      `json:"created_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

type GoogleAuth struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	AccessToken  string    `json:"-"`
	RefreshToken string    `json:"-"`
	Expiry       time.Time `json:"expiry"`
}
