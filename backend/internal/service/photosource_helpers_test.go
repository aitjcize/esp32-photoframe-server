package service

import (
	"image"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/imagesource"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

func mkOrientedImage(t *testing.T, db *gorm.DB, ext, orientation string) model.Image {
	t.Helper()
	img := model.Image{
		ExternalID:  ext,
		Source:      model.SourceImmich,
		Status:      "pending",
		Orientation: orientation,
	}
	if err := db.Create(&img).Error; err != nil {
		t.Fatalf("create image: %v", err)
	}
	return img
}

func dbPicker(db *gorm.DB) PhotoPicker {
	return func(orientation string, exclude []uint) (model.Image, error) {
		return PickRandomDBPhoto(db, model.SourceImmich, orientation, exclude)
	}
}

func stubLoader(model.Image) (image.Image, error) {
	return image.NewRGBA(image.Rect(0, 0, 10, 10)), nil
}

type pickCall struct {
	orientation string
	excludeLen  int
}

// The requested orientation must reach the picker on the first attempt.
func TestPickRandomWithFallback_PassesOrientation(t *testing.T) {
	var seen []pickCall
	pick := func(orientation string, exclude []uint) (model.Image, error) {
		seen = append(seen, pickCall{orientation, len(exclude)})
		return model.Image{ID: 1, Orientation: orientation}, nil
	}
	item, err := pickRandomWithFallback(pick, "portrait", []uint{2, 3})
	assert.NoError(t, err)
	assert.Equal(t, "portrait", item.Orientation)
	assert.Equal(t, []pickCall{{"portrait", 2}}, seen)
}

// An empty orientation+exclusions result retries the *same* orientation without
// exclusions before ever relaxing the orientation.
func TestPickRandomWithFallback_DropsExclusionsBeforeOrientation(t *testing.T) {
	var seen []pickCall
	pick := func(orientation string, exclude []uint) (model.Image, error) {
		seen = append(seen, pickCall{orientation, len(exclude)})
		if orientation == "portrait" && len(exclude) == 0 {
			return model.Image{ID: 5, Orientation: "portrait"}, nil
		}
		return model.Image{}, gorm.ErrRecordNotFound
	}
	item, err := pickRandomWithFallback(pick, "portrait", []uint{1, 2})
	assert.NoError(t, err)
	assert.Equal(t, uint(5), item.ID)
	assert.Equal(t, "portrait", item.Orientation)
	assert.Equal(t, []pickCall{{"portrait", 2}, {"portrait", 0}}, seen)
}

// Only when no photo of the requested orientation exists does it fall back to
// any orientation.
func TestPickRandomWithFallback_FallsBackToAnyOrientation(t *testing.T) {
	var seen []pickCall
	pick := func(orientation string, exclude []uint) (model.Image, error) {
		seen = append(seen, pickCall{orientation, len(exclude)})
		if orientation == "" {
			return model.Image{ID: 9, Orientation: "landscape"}, nil
		}
		return model.Image{}, gorm.ErrRecordNotFound
	}
	item, err := pickRandomWithFallback(pick, "portrait", []uint{1})
	assert.NoError(t, err)
	assert.Equal(t, uint(9), item.ID)
	// portrait+exclude → portrait+none → any(none).
	assert.Equal(t, []pickCall{{"portrait", 1}, {"portrait", 0}, {"", 0}}, seen)
}

// End-to-end via RunDBPhotoFlow: a portrait device is never served a landscape
// photo while portrait photos exist.
func TestRunDBPhotoFlow_PrefersDeviceOrientation(t *testing.T) {
	db := setupAlbumDB(t)
	mkOrientedImage(t, db, "p1", "portrait")
	mkOrientedImage(t, db, "p2", "portrait")
	mkOrientedImage(t, db, "l1", "landscape")

	req := &imagesource.Request{Orientation: "portrait", Width: 480, Height: 800}
	for i := 0; i < 40; i++ {
		resp, err := RunDBPhotoFlow(req, db, dbPicker(db), stubLoader)
		assert.NoError(t, err)
		if assert.Len(t, resp.ImageIDs, 1) {
			var img model.Image
			assert.NoError(t, db.First(&img, resp.ImageIDs[0]).Error)
			assert.Equal(t, "portrait", img.Orientation,
				"portrait device should not be served a landscape photo")
		}
	}
}

// "auto" photos (no EXIF dimensions) match any device orientation.
func TestRunDBPhotoFlow_AutoMatchesAnyOrientation(t *testing.T) {
	db := setupAlbumDB(t)
	mkOrientedImage(t, db, "a1", "auto")

	req := &imagesource.Request{Orientation: "portrait", Width: 480, Height: 800}
	resp, err := RunDBPhotoFlow(req, db, dbPicker(db), stubLoader)
	assert.NoError(t, err)
	assert.Len(t, resp.ImageIDs, 1)
}

// When only mismatched-orientation photos exist, still return one rather than
// failing — a cropped photo beats a blank frame.
func TestRunDBPhotoFlow_FallbackWhenNoOrientationMatch(t *testing.T) {
	db := setupAlbumDB(t)
	mkOrientedImage(t, db, "l1", "landscape") // only landscape available

	req := &imagesource.Request{Orientation: "portrait", Width: 480, Height: 800}
	resp, err := RunDBPhotoFlow(req, db, dbPicker(db), stubLoader)
	assert.NoError(t, err)
	assert.Len(t, resp.ImageIDs, 1)
}
