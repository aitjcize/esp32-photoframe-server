package service

// A frame shows exactly what its owner chose: a source, perhaps one
// connection of it, perhaps some albums of that. Whatever a frame currently
// draws from cannot be removed while it does, because the frame would
// silently move to a wider pool (every connection of the provider, the whole
// gallery). The owner points the frame elsewhere first.
//
// A frame's album selections are remembered per source and per connection,
// so it can be switched away and back without picking again. Selections for
// a source or connection the frame is not on are inert: they do not hold up
// a deletion, and the deletion clears them (the mappings cascade with the
// albums).

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

// InUseError says what could not be removed and which frames draw from it.
type InUseError struct {
	What    string // e.g. `the connection "Home"`, `the album "Trips"`
	Devices []model.Device
}

func (e *InUseError) Error() string {
	return fmt.Sprintf("%s is used by %s: %s. Change their Image Source first.",
		e.What, plural(len(e.Devices), "frame"), strings.Join(e.DeviceNames(), ", "))
}

// DeviceNames lists the frames by name (host when unnamed), in id order.
func (e *InUseError) DeviceNames() []string {
	names := make([]string, 0, len(e.Devices))
	for _, d := range e.Devices {
		if d.Name != "" {
			names = append(names, d.Name)
		} else {
			names = append(names, d.Host)
		}
	}
	return names
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// FramesUsingConnection returns the frames that draw from the connection:
// bound to it as their image source, showing its calendar (a frame with the
// calendar switched off only remembers the account), or on "any connection"
// of its source with one of its albums selected (its loss would change what
// they show).
func FramesUsingConnection(db *gorm.DB, conn model.Connection) ([]model.Device, error) {
	var devices []model.Device
	err := db.Model(&model.Device{}).
		Where("image_connection_id = ? OR (calendar_connection_id = ? AND show_calendar = ?)", conn.ID, conn.ID, true).
		Or("source = ? AND image_connection_id IS NULL AND id IN (?)", conn.Source,
			db.Model(&model.DeviceAlbumMapping{}).Select("device_album_mappings.device_id").
				Joins("JOIN albums ON albums.id = device_album_mappings.album_id").
				Where("albums.connection_id = ?", conn.ID)).
		Order("id").Find(&devices).Error
	return devices, err
}

// FramesUsingAlbums returns the frames that draw from any of the albums: on
// the album's source, and on its connection or on "any connection".
func FramesUsingAlbums(db *gorm.DB, albumIDs []uint) ([]model.Device, error) {
	if len(albumIDs) == 0 {
		return nil, nil
	}
	var devices []model.Device
	err := db.Model(&model.Device{}).
		Where("id IN (?)", db.Model(&model.DeviceAlbumMapping{}).Select("device_album_mappings.device_id").
			Joins("JOIN albums ON albums.id = device_album_mappings.album_id").
			Joins("JOIN devices d ON d.id = device_album_mappings.device_id").
			Where("device_album_mappings.album_id IN ?", albumIDs).
			Where("d.source = albums.source").
			Where("d.image_connection_id IS NULL OR albums.connection_id IS NULL OR d.image_connection_id = albums.connection_id")).
		Order("id").Find(&devices).Error
	return devices, err
}
