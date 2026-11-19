package service

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/weather"
)

// The photo date is its own overlay option: every layout shows it whether or
// not today's date is shown, and with or without weather. (Photo + Info Strip
// and Side Panel used to show it only together with Show Date, leaving an
// empty strip when it was the only option on.)
func TestEveryLayoutShowsThePhotoDateOnItsOwn(t *testing.T) {
	s, err := NewRendererService()
	if err != nil {
		t.Fatal(err)
	}
	const photoDate = "Jul 04, 2019"
	const today = "Thu, Nov 19"
	for _, layout := range []string{"photo_info", "photo_overlay", "side_panel", ""} {
		for _, showDate := range []bool{false, true} {
			for _, showWeather := range []bool{false, true} {
				data := templateData{
					Layout: layout, Width: 800, Height: 480, BaseUnit: 4.8, PhotoRatio: 0.8,
					ShowDate: showDate, DateStr: today,
					ShowPhotoDate: true, PhotoDateStr: photoDate,
					ShowWeather: showWeather,
				}
				if showWeather {
					data.Weather = &weather.CurrentWeather{Temperature: 21.5, Humidity: 40}
				}
				var buf bytes.Buffer
				if err := s.tmpl.Execute(&buf, data); err != nil {
					t.Fatalf("layout %q: %v", layout, err)
				}
				html := buf.String()
				name := "layout=" + layout
				if showDate {
					name += " +date"
				}
				if showWeather {
					name += " +weather"
				}
				if !strings.Contains(html, photoDate) {
					t.Errorf("%s: photo date missing", name)
				}
				if got := strings.Contains(html, today); got != showDate {
					t.Errorf("%s: today's date shown = %v, want %v", name, got, showDate)
				}
			}
		}
	}
}
