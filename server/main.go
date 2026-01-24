package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/aitjcize/photoframe-server/server/internal/db"
	"github.com/aitjcize/photoframe-server/server/internal/handler"
	"github.com/aitjcize/photoframe-server/server/internal/service"
	"github.com/aitjcize/photoframe-server/server/pkg/googlephotos"
	"github.com/aitjcize/photoframe-server/server/pkg/weather"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Initialize Database
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "photoframe.db"
	}
	database, err := db.Init(dbPath)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// Initialize Services
	settingsService := service.NewSettingsService(database)
	tokenStore := service.NewDBTokenStore(database)

	// Initialize Google Client
	// Pass settingsService as ConfigProvider so it fetches latest config on every request
	googleClient := googlephotos.NewClient(settingsService, tokenStore)

	// Initialize Processor
	// We use global photoframe-process command
	processorService := service.NewProcessorService()
	// Initialize Overlay
	weatherClient := weather.NewClient()
	overlayService := service.NewOverlayService(weatherClient, settingsService)

	// Initialize Picker Service
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	cleanupTempThumbnails(dataDir)

	pickerService := service.NewPickerService(googleClient, database, dataDir)

	// Initialize Telegram Service
	telegramService := service.NewTelegramService(database, dataDir)
	telegramToken, _ := settingsService.Get("telegram_bot_token")
	if telegramToken != "" {
		telegramService.Restart(telegramToken)
	}

	// Initialize Handlers
	h := handler.NewHandler(settingsService, telegramService)
	gh := handler.NewGoogleHandler(googleClient, pickerService)
	ih := handler.NewImageHandler(settingsService, overlayService, processorService, googleClient, database, dataDir)

	// Echo instance
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:5173"},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))

	// Routes
	api := e.Group("/api")
	api.GET("/status", h.HealthCheck)
	api.GET("/settings", h.GetSettings)
	api.POST("/settings", h.UpdateSettings)

	// Image Route
	e.GET("/image", ih.ServeImage)

	// Google Routes
	api.GET("/auth/google/login", gh.Login)
	api.GET("/auth/google/callback", gh.Callback)
	// api.GET("/google/albums", gh.ListAlbums) // Deprecated

	// Picker Routes
	api.GET("/google/picker/session", gh.CreatePickerSession)
	api.GET("/google/picker/poll/:id", gh.PollPickerSession)
	api.GET("/google/picker/progress/:id", gh.PollPickerProgress)
	api.POST("/google/picker/process/:id", gh.ProcessPickerSession)

	// Gallery Routes
	api.GET("/photos", ih.ListPhotos)
	api.DELETE("/photos/:id", ih.DeletePhoto)
	api.GET("/photos/:id/thumbnail", ih.GetThumbnail)
	api.GET("/served-image-thumbnail/:id", ih.GetServedImageThumbnail)

	// Static Files (Frontend)
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = "./static"
	}

	// 1. Serve specific assets folder
	// This handles /assets/index-....js|css correctly with proper MIME types
	e.Static("/assets", filepath.Join(staticDir, "assets"))

	// 2. Serve root index.html
	e.File("/", filepath.Join(staticDir, "index.html"))

	// 3. SPA Fallback: Any other route not matched (api is already handled) goes to index.html
	e.GET("/*", func(c echo.Context) error {
		return c.File(filepath.Join(staticDir, "index.html"))
	})

	// Start server
	e.Logger.Fatal(e.Start(":9607"))
}

func cleanupTempThumbnails(dataDir string) {
	pattern := filepath.Join(dataDir, "thumb_*.jpg")
	files, err := filepath.Glob(pattern)
	if err != nil {
		log.Printf("Failed to list temp thumbnails for cleanup: %v", err)
		return
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			log.Printf("Failed to remove temp thumbnail %s: %v", f, err)
		} else {
			log.Printf("Cleaned up temp thumbnail: %s", f)
		}
	}
}
