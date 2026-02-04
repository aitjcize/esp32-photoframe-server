package weather

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type Weather struct {
	Current CurrentWeather `json:"current_weather"`
	Hourly  HourlyWeather  `json:"hourly"`
}

type CurrentWeather struct {
	Temperature float64 `json:"temperature"`
	WeatherCode int     `json:"weathercode"`
	Time        string  `json:"time"`
	Humidity    int     // Extracted from hourly data
}

type HourlyWeather struct {
	Time               []string `json:"time"`
	RelativeHumidity2m []int    `json:"relativehumidity_2m"`
	WeatherCode        []int    `json:"weathercode"`
}

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{}}
}

func (c *Client) GetWeather(lat, lon string) (*CurrentWeather, error) {
	// Request hourly data for precise humidity and weather matching
	url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s&current_weather=true&hourly=temperature_2m,relativehumidity_2m,weathercode&forecast_days=1", lat, lon)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weather api returned status: %d", resp.StatusCode)
	}

	var result Weather
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// 1. Find the index in hourly data that matches the current weather time
	// This ensures we get the humidity/icon for the *current* hour, not midnight
	targetTime := result.Current.Time
	idx := 0
	found := false

	// Check if we have hourly times
	if len(result.Hourly.Time) > 0 {
		for i, t := range result.Hourly.Time {
			if t == targetTime {
				idx = i
				found = true
				break
			}
		}
	}

	// 2. Populate CurrentWeather
	if found {
		// Use hourly data for consistency if match found
		if idx < len(result.Hourly.RelativeHumidity2m) {
			result.Current.Humidity = result.Hourly.RelativeHumidity2m[idx]
		}
		if idx < len(result.Hourly.WeatherCode) {
			result.Current.WeatherCode = result.Hourly.WeatherCode[idx]
		}
		// We could also use temperature_2m from hourly, but current_weather.temperature is usually fine
	} else if len(result.Hourly.RelativeHumidity2m) > 0 {
		// Fallback to first item if no time match (shouldn't happen often)
		result.Current.Humidity = result.Hourly.RelativeHumidity2m[0]
	}

	return &result.Current, nil
}

func (c CurrentWeather) Description() string {
	switch c.WeatherCode {
	case 0:
		return "Clear"
	case 1, 2, 3:
		return "Cloudy"
	case 45, 48:
		return "Fog"
	case 51, 53, 55, 56, 57:
		return "Drizzle"
	case 61, 63, 65, 66, 67:
		return "Rain"
	case 71, 73, 75, 77:
		return "Snow"
	case 80, 81, 82:
		return "Showers"
	case 85, 86:
		return "Snow Showers"
	case 95, 96, 99:
		return "Thunderstorm"
	default:
		return "Unknown"
	}
}

// Icon returns a Material Symbols icon code based on the weather code
func (c CurrentWeather) Icon() string {
	switch c.WeatherCode {
	case 0:
		return "\uf157" // clear_day
	case 1, 2:
		return "\uf172" // partly_cloudy_day
	case 3, 45, 48:
		return "\uf15c" // cloud
	case 51, 53, 55, 61, 63, 65, 80, 81, 82:
		return "\uf176" // rainy
	case 56, 57, 66, 67:
		return "\uf176" // weather_mix -> fallback to rainy or similar symbols symbol
	case 71, 73, 75, 77, 85, 86:
		return "\ueb3b" // ac_unit (snow)
	case 95, 96, 99:
		return "\uebdb" // thunderstorm
	default:
		return "\uf157" // clear_day (default)
	}
}
