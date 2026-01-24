package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/aitjcize/photoframe-server/server/internal/db"
	"github.com/aitjcize/photoframe-server/server/internal/handler"
	"github.com/aitjcize/photoframe-server/server/internal/middleware"
	"github.com/aitjcize/photoframe-server/server/internal/model"
	"github.com/aitjcize/photoframe-server/server/internal/service"
	"github.com/aitjcize/photoframe-server/server/pkg/googlephotos"
	"github.com/aitjcize/photoframe-server/server/pkg/photoframe"
	"github.com/aitjcize/photoframe-server/server/pkg/weather"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
)

func main() {
	// Initialize Database
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "esp32-photoframe/photoframe.db"
	}

	// Ensure directory exists for dbPath
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatalf("Failed to create database directory: %v", err)
	}

	database, err := db.Init(dbPath)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// Initialize Services
	settingsService := service.NewSettingsService(database)
	tokenStore := service.NewDBTokenStore(database)
	// JWT Secret - In production, this should come from env but for Addon we might generate or fix it
	jwtSecret := os.Getenv("JWT_SECRET")
	authService := service.NewAuthService(database, jwtSecret)

	// Migrate User Model
	if err := database.AutoMigrate(&model.User{}, &model.APIKey{}); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	// Initialize Google Client
	// Pass settingsService as ConfigProvider so it fetches latest config on every request
	googleClient := googlephotos.NewClient(settingsService, tokenStore)

	// Initialize Processor
	// We use global photoframe-process command
	processorService := service.NewProcessorService()
	// Initialize Overlay
	weatherClient := weather.NewClient()
	overlayService := service.NewOverlayService(weatherClient, settingsService)
	// Initialize Synology Photos Service
	synologyService := service.NewSynologyService(database, settingsService)

	// Initialize Picker Service
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "esp32-photoframe/data"
	}
	// Ensure dataDir exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	cleanupTempThumbnails(dataDir)

	pickerService := service.NewPickerService(googleClient, database, dataDir)

	// Initialize PhotoFrame Client
	photoframeClient := photoframe.NewClient()

	// Initialize Telegram Service
	telegramService := service.NewTelegramService(database, dataDir, processorService, settingsService, photoframeClient, overlayService)
	telegramToken, _ := settingsService.Get("telegram_bot_token")
	if telegramToken != "" {
		telegramService.Restart(telegramToken)
	}

	// Initialize Handlers
	h := handler.NewHandler(settingsService, telegramService, googleClient)
	gh := handler.NewGoogleHandler(googleClient, pickerService, database, dataDir)
	sh := handler.NewSynologyHandler(synologyService)
	ih := handler.NewImageHandler(settingsService, overlayService, processorService, googleClient, synologyService, database, dataDir)
	ah := handler.NewAuthHandler(authService)

	// Echo instance
	e := echo.New()

	// Middleware
	e.Use(echoMiddleware.Logger())
	e.Use(echoMiddleware.Recover())
	e.Use(echoMiddleware.CORSWithConfig(echoMiddleware.CORSConfig{
		AllowOrigins: []string{"http://localhost:5173", "http://homeassistant.local:8123"},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
	}))

	// Auth Middleware
	authMiddleware := middleware.JWTMiddleware(authService)

	// Public Auth Routes
	apiAuth := e.Group("/api/auth")
	apiAuth.POST("/login", ah.Login)
	apiAuth.POST("/register", ah.Register)
	apiAuth.GET("/status", ah.GetStatus)

	// Auth Management (Tokens) - Protected
	// We attach these to protectedApi below, but conceptually they are auth related

	// Public Health Check
	e.GET("/api/status", h.HealthCheck)
	// Public Serve Thumbnail/Image (Actually Request says image endpoint SHOULD be protected)
	// The user requested /image/:source to be protected.
	// We need to support ?token= or Authorization header.

	// Image Route (Protected)
	e.GET("/image/:source", ih.ServeImage, authMiddleware)
	// Thumbnail likely needs protection too, or obscure IDs. For now, keep public as they are temporary?
	// User said "access the /image/<source>/ endpoint. This one... people can't just access".
	// Let's protect main image endpoint.
	e.GET("/served-image-thumbnail/:id", ih.GetServedImageThumbnail)

	// Protected API Routes
	// 1. Protected API Group
	protectedApi := e.Group("/api", authMiddleware)
	protectedApi.GET("/settings", h.GetSettings)
	protectedApi.GET("/settings", h.GetSettings)
	protectedApi.POST("/settings", h.UpdateSettings)

	// Device Tokens (Protected)
	protectedApi.POST("/auth/tokens", ah.GenerateDeviceToken)
	protectedApi.GET("/auth/tokens", ah.ListTokens)
	protectedApi.DELETE("/auth/tokens/:id", ah.RevokeToken)
	protectedApi.POST("/auth/password", ah.ChangePassword)

	// Google Picker (Protected)
	protectedApi.GET("/google/picker/session", gh.CreatePickerSession)
	protectedApi.GET("/google/picker/poll/:id", gh.PollPickerSession)
	protectedApi.GET("/google/picker/progress/:id", gh.PollPickerProgress)
	protectedApi.POST("/google/picker/process/:id", gh.ProcessPickerSession)

	// Gallery (Protected)
	protectedApi.GET("/google-photos", gh.ListGooglePhotos)
	protectedApi.DELETE("/google-photos", gh.DeleteAllGooglePhotos)
	protectedApi.DELETE("/google-photos/:id", gh.DeleteGooglePhoto)
	protectedApi.GET("/google-photos/:id/thumbnail", gh.GetGooglePhotoThumbnail)

	// Synology (Protected)
	protectedApi.POST("/synology/test", sh.TestConnection)
	protectedApi.POST("/synology/sync", sh.Sync)
	protectedApi.POST("/synology/clear", sh.Clear)
	protectedApi.GET("/synology/albums", sh.ListAlbums)
	protectedApi.GET("/synology/count", sh.GetPhotoCount)
	protectedApi.POST("/synology/logout", sh.Logout)

	// Google Auth: Login (Protected - User initiates), Callback (Public - Google calls)
	protectedApi.GET("/auth/google/login", gh.Login)
	protectedApi.POST("/auth/google/logout", gh.Logout)

	// Public Callback
	e.GET("/api/auth/google/callback", gh.Callback)

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
