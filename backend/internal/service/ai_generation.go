package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
)

type AIGenerationService struct {
	settings *SettingsService
}

func NewAIGenerationService(settings *SettingsService) *AIGenerationService {
	return &AIGenerationService{settings: settings}
}

func (s *AIGenerationService) Generate(device *model.Device) (image.Image, error) {
	fmt.Printf("AI Generation: device=%s, provider=%s, model=%s, prompt=%s\n",
		device.Name, device.AIProvider, device.AIModel, device.AIPrompt)

	if device.AIPrompt == "" {
		return nil, fmt.Errorf("AI prompt not configured for device %s", device.Name)
	}

	provider := device.AIProvider
	modelName := device.AIModel

	if provider == "" {
		return nil, fmt.Errorf("AI provider not configured for device %s", device.Name)
	}
	if modelName == "" {
		return nil, fmt.Errorf("AI model not configured for device %s", device.Name)
	}

	isPortrait := device.Height > device.Width
	if device.Orientation == "portrait" {
		isPortrait = true
	} else if device.Orientation == "landscape" {
		isPortrait = false
	}

	switch provider {
	case "openai":
		return s.generateOpenAI(device.AIPrompt, modelName, isPortrait)
	case "google":
		return s.generateGemini(device.AIPrompt, modelName, isPortrait, device.Width, device.Height)
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", provider)
	}
}

func (s *AIGenerationService) generateOpenAI(prompt, modelName string, isPortrait bool) (image.Image, error) {
	apiKey, err := s.settings.Get("openai_api_key")
	if err != nil || apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key not configured")
	}

	isDalle3 := strings.Contains(modelName, "dall-e-3")
	isDalle2 := strings.Contains(modelName, "dall-e-2")

	size := "1024x1024"
	if isDalle3 {
		if isPortrait {
			size = "1024x1792"
		} else {
			size = "1792x1024"
		}
	} else if isDalle2 {
		size = "1024x1024"
	} else {
		// GPT Image models
		if isPortrait {
			size = "1024x1536"
		} else {
			size = "1536x1024"
		}
	}

	body := map[string]interface{}{
		"model":  modelName,
		"prompt": prompt,
		"n":      1,
		"size":   size,
	}

	if isDalle3 {
		body["quality"] = "hd"
		body["style"] = "vivid"
		body["response_format"] = "b64_json"
	} else if isDalle2 {
		body["response_format"] = "b64_json"
	} else {
		body["quality"] = "high"
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	fmt.Printf("OpenAI request: %s\n", string(jsonBody))

	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/images/generations", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	fmt.Printf("OpenAI response status: %d\n", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("OpenAI error response: %s\n", string(respBody))
		return nil, fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("no image data in OpenAI response")
	}

	var imgData []byte
	if result.Data[0].B64JSON != "" {
		imgData, err = base64.StdEncoding.DecodeString(result.Data[0].B64JSON)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 image: %w", err)
		}
	} else if result.Data[0].URL != "" {
		imgResp, err := client.Get(result.Data[0].URL)
		if err != nil {
			return nil, fmt.Errorf("failed to download image from URL: %w", err)
		}
		defer imgResp.Body.Close()
		imgData, err = io.ReadAll(imgResp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read image data: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no image data in OpenAI response")
	}

	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	return img, nil
}

func (s *AIGenerationService) generateGemini(prompt, modelName string, isPortrait bool, width, height int) (image.Image, error) {
	apiKey, err := s.settings.Get("google_api_key")
	if err != nil || apiKey == "" {
		return nil, fmt.Errorf("Google API key not configured")
	}

	aspectRatio := "4:3"
	if isPortrait {
		aspectRatio = "3:4"
	}

	imageConfig := map[string]interface{}{
		"aspectRatio": aspectRatio,
	}

	if strings.Contains(modelName, "gemini-3") {
		maxDim := width
		if height > maxDim {
			maxDim = height
		}
		if maxDim > 2048 {
			imageConfig["imageSize"] = "4K"
		} else if maxDim > 1024 {
			imageConfig["imageSize"] = "2K"
		} else {
			imageConfig["imageSize"] = "1K"
		}
	}

	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"Image"},
			"imageConfig":        imageConfig,
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", modelName, apiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Gemini API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData struct {
						Data     string `json:"data"`
						MimeType string `json:"mimeType"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no image data in Gemini response")
	}

	b64Data := result.Candidates[0].Content.Parts[0].InlineData.Data
	if b64Data == "" {
		return nil, fmt.Errorf("empty image data in Gemini response")
	}

	imgData, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 image: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	return img, nil
}
