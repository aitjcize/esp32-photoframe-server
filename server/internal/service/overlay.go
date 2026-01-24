package service

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/aitjcize/photoframe-server/server/pkg/weather"
	"github.com/fogleman/gg"
)

type OverlayService struct {
	weatherClient *weather.Client
	settings      *SettingsService
}

func NewOverlayService(w *weather.Client, s *SettingsService) *OverlayService {
	return &OverlayService{weatherClient: w, settings: s}
}

func (s *OverlayService) ApplyOverlay(img image.Image) (image.Image, error) {
	// 1. Check if we need to draw anything at all
	showDate, _ := s.settings.Get("show_date")
	if showDate == "" {
		showDate = "true"
	}
	showWeather, _ := s.settings.Get("show_weather")
	if showWeather == "" {
		showWeather = "true"
	}
	lat, _ := s.settings.Get("weather_lat")
	lon, _ := s.settings.Get("weather_lon")

	hasDate := showDate == "true"
	hasWeather := showWeather == "true" && lat != "" && lon != ""

	if !hasDate && !hasWeather {
		return img, nil
	}

	dc := gg.NewContextForImage(img)
	w := float64(dc.Width())
	h := float64(dc.Height())

	// 2. Draw Gradient (only if we have overlays)
	gradientHeight := 150.0
	grad := gg.NewLinearGradient(0, h-gradientHeight, 0, h)
	grad.AddColorStop(0, color.RGBA{0, 0, 0, 0})
	grad.AddColorStop(0.3, color.RGBA{0, 0, 0, 120})
	grad.AddColorStop(0.6, color.RGBA{0, 0, 0, 200})
	grad.AddColorStop(1, color.RGBA{0, 0, 0, 255})

	dc.SetFillStyle(grad)
	dc.DrawRectangle(0, h-gradientHeight, w, gradientHeight)
	dc.Fill()

	// 3. Load Font
	fontPaths := []string{
		"/usr/share/fonts/noto/NotoSans-Regular.ttf",   // Linux (Docker)
		"../bin/fonts/NotoSans-Regular.ttf",            // Local dev
		"/System/Library/Fonts/Supplemental/Arial.ttf", // macOS fallback
		"/Library/Fonts/Arial.ttf",                     // macOS alternative
	}
	var validFontPath string
	for _, fontPath := range fontPaths {
		if err := dc.LoadFontFace(fontPath, 25); err == nil {
			validFontPath = fontPath
			break
		}
	}
	if validFontPath == "" {
		fmt.Printf("Warning: Could not load any font, text overlay will not work\n")
		return img, nil
	}

	// 4. Configuration for text positioning
	marginBottom := 50.0

	// Date
	if hasDate {
		now := time.Now()
		dateStr := now.Format("Mon, Jan 02")

		x := 20.0
		y := h - marginBottom

		// Draw text
		dc.SetRGB(1, 1, 1)
		dc.DrawStringAnchored(dateStr, x, y, 0, 0.5)
	}

	// Weather
	if hasWeather {
		weather, err := s.weatherClient.GetWeather(lat, lon)
		if err == nil {
			// Draw large weather icon using Material Symbols font
			iconFontPaths := []string{
				"/usr/share/fonts/material/MaterialSymbolsOutlined.ttf",
				"../bin/fonts/MaterialSymbolsOutlined.ttf",
			}
			iconFontLoaded := false
			for _, p := range iconFontPaths {
				if err := dc.LoadFontFace(p, 72); err == nil {
					iconFontLoaded = true
					break
				}
			}

			if iconFontLoaded {
				icon := weather.Icon()
				iconX := w - 20
				// Icon sits above the text.
				// Text is at h - marginBottom.
				// Icon center previously at h - 60 (diff 35).
				iconY := h - marginBottom - 35

				// Draw icon
				dc.SetRGB(1, 1, 1)
				dc.DrawStringAnchored(icon, iconX, iconY, 1, 0.5)
			}
			// If Material Symbols font not available, skip weather icon (text will still show)

			// Draw temperature and humidity below in smaller text using detected font
			if err := dc.LoadFontFace(validFontPath, 18); err == nil {
				weatherStr := fmt.Sprintf("%.1f°C  %d%%", weather.Temperature, weather.Humidity)
				textX := w - 20
				textY := h - marginBottom

				// Draw text
				dc.SetRGB(1, 1, 1)
				dc.DrawStringAnchored(weatherStr, textX, textY, 1, 0.5)
			}
		} else {
			// If weather fails, silently skip or maybe log
			fmt.Printf("Weather fetch failed: %v\n", err)
		}
	}

	return dc.Image(), nil
}
