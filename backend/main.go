package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/db"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/handler"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/middleware"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/photoframe"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/weather"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
)

func main() {
	// Initialize Database
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "esp32-photoframe/photoframe.db"
	}

	// Migration Logic: Check if legacy DB exists in /config and new DB doesn't exist
	// This is specific to HA Add-on migration
	legacyDBPath := "/config/esp32-photoframe-server/photoframe.db"
	if dbPath == "/data/photoframe.db" {
		if _, err := os.Stat(legacyDBPath); err == nil {
			if _, err := os.Stat(dbPath); os.IsNotExist(err) {
				log.Println("Migrating database from legacy path:", legacyDBPath)
				if err := os.Rename(legacyDBPath, dbPath); err != nil {
					log.Printf("Failed to migrate database: %v", err)
					// Try copying if rename fails (start across filesystems)
					input, err := os.ReadFile(legacyDBPath)
					if err == nil {
						err = os.WriteFile(dbPath, input, 0644)
						if err == nil {
							log.Println("Database copied successfully")
							os.Remove(legacyDBPath)
						} else {
							log.Printf("Failed to copy database: %v", err)
						}
					}
				} else {
					log.Println("Database migration successful")
				}
			}
		}
	}

	// Data Directory Migration for Add-on
	// Check if legacy data directory exists and new data directory is /data
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "esp32-photoframe/data"
	}
	legacyDataDir := "/config/esp32-photoframe-server/data"

	if dataDir == "/data" {
		if info, err := os.Stat(legacyDataDir); err == nil && info.IsDir() {
			log.Println("Found legacy data directory, attempting migration to:", dataDir)

			// Use pure Go for copying to ensure compatibility with BusyBox
			// BusyBox cp doesn't support -n flag
			err := filepath.Walk(legacyDataDir, func(srcPath string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				// Calculate relative path
				relPath, err := filepath.Rel(legacyDataDir, srcPath)
				if err != nil {
					return err
				}
				dstPath := filepath.Join(dataDir, relPath)

				if info.IsDir() {
					return os.MkdirAll(dstPath, info.Mode())
				}

				// Skip if destination already exists (no-clobber behavior)
				if _, err := os.Stat(dstPath); err == nil {
					log.Printf("Skipping %s (already exists)", relPath)
					return nil
				}

				// Copy file
				input, err := os.ReadFile(srcPath)
				if err != nil {
					return err
				}
				if err := os.WriteFile(dstPath, input, info.Mode()); err != nil {
					return err
				}
				log.Printf("Copied %s", relPath)
				return nil
			})

			if err != nil {
				log.Printf("Failed to migrate data directory: %v", err)
			} else {
				log.Println("Data directory migration successful")
				log.Println("Please manually verify and remove legacy data in " + legacyDataDir)
			}
		}
	}

	// Ensure directory exists for dbPath
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatalf("Failed to create database directory: %v", err)
	}

	database, err := db.Init(dbPath)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// Run Migrations
	if err := db.Migrate(database, dbPath); err != nil {
		log.Fatal("Failed to run database migrations:", err)
	}

	// Initialize Services
	settingsService := service.NewSettingsService(database)
	tokenStore := service.NewDBTokenStore(database)
	// JWT Secret - In production, this should come from env but for Addon we might generate or fix it
	jwtSecret := os.Getenv("JWT_SECRET")
	authService := service.NewAuthService(database, jwtSecret)

	// Migrate All Models
	// Device and other models are handled by golang-migrate now
	/*
		if err := database.AutoMigrate(
			&model.User{},
			&model.APIKey{},
			&model.Setting{},
			&model.Image{},
			&model.GoogleAuth{},
		); err != nil {
			log.Fatal("Failed to migrate database:", err)
		}
	*/

	// Initialize Google Client
	// Pass settingsService as ConfigProvider so it fetches latest config on every request
	googleClient := googlephotos.NewClient(settingsService, tokenStore)

	// Initialize Processor
	// We use global epaper-image-convert command
	processorService := service.NewProcessorService()
	// Initialize Overlay
	weatherClient := weather.NewClient()
	overlayService := service.NewOverlayService(weatherClient, settingsService)
	// Initialize Synology Photos Service
	synologyService := service.NewSynologyService(database, settingsService)

	// Initialize Picker Service
	// dataDir already set from migration logic above
	// Ensure dataDir exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	cleanupTempThumbnails(dataDir)

	pickerService := service.NewPickerService(googleClient, database, dataDir)

	// Initialize PhotoFrame Client
	photoframeClient := photoframe.NewClient()

	// Initialize Device Service
	deviceService := service.NewDeviceService(database, settingsService, processorService, overlayService, photoframeClient)
	deviceHandler := handler.NewDeviceHandler(deviceService, synologyService, authService, settingsService, database)

	// Initialize Telegram Service
	// Pass deviceService as Pusher
	telegramService := service.NewTelegramService(database, dataDir, settingsService, deviceService)
	// telegramHandler removed as it does not exist
	// Start bot: now deferred to start after config load or handled within service constructor
	// telegramService.StartBot() // Removed auto-start here, service handles it if token exists

	telegramToken, _ := settingsService.Get("telegram_bot_token")
	if telegramToken != "" {
		telegramService.Restart(telegramToken)
	}

	// Initialize Handlers
	h := handler.NewHandler(settingsService, telegramService, googleClient)
	googleHandler := handler.NewGoogleHandler(googleClient, pickerService, database, dataDir)
	sh := handler.NewSynologyHandler(synologyService)
	// Reuse 'gh' variable name for GalleryHandler because I used 'gh' in routes above.
	// Wait, 'gh' was GoogleHandler before. I should rename GoogleHandler to 'googleHandler' and 'gh' to GalleryHandler to match my routes change.
	gh := handler.NewGalleryHandler(database, synologyService, dataDir)
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

	// Device Management (Protected)
	protectedApi.GET("/devices", deviceHandler.ListDevices)
	protectedApi.POST("/devices", deviceHandler.AddDevice)
	protectedApi.PUT("/devices/:id", deviceHandler.UpdateDevice)
	protectedApi.DELETE("/devices/:id", deviceHandler.DeleteDevice)
	protectedApi.POST("/devices/:id/push", deviceHandler.PushToDevice)
	protectedApi.POST("/devices/:id/configure-source", deviceHandler.ConfigureDeviceSource)

	// Device Tokens (Protected)
	protectedApi.POST("/auth/tokens", ah.GenerateDeviceToken)
	protectedApi.GET("/auth/tokens", ah.ListTokens)
	protectedApi.DELETE("/auth/tokens/:id", ah.RevokeToken)
	protectedApi.GET("/auth/sessions", ah.ListSessions)
	protectedApi.DELETE("/auth/sessions/:id", ah.RevokeSession)
	protectedApi.POST("/auth/account", ah.UpdateAccount)

	// Gallery (Protected) - Unified
	protectedApi.GET("/gallery/photos", gh.ListPhotos)
	protectedApi.GET("/gallery/thumbnail/:id", gh.GetThumbnail)
	protectedApi.DELETE("/gallery/photos/:id", gh.DeletePhoto)
	protectedApi.DELETE("/gallery/photos", gh.DeletePhotos)
	// URL Proxy
	protectedApi.POST("/gallery/urls", gh.CreateURLSource)
	protectedApi.GET("/gallery/urls", gh.ListURLSources)
	protectedApi.PUT("/gallery/urls/:id", gh.UpdateURLSource)
	protectedApi.DELETE("/gallery/urls/:id", gh.DeleteURLSource)

	// Google Picker (Protected)
	protectedApi.GET("/google/picker/session", googleHandler.CreatePickerSession)
	protectedApi.GET("/google/picker/poll/:id", googleHandler.PollPickerSession)
	protectedApi.GET("/google/picker/progress/:id", googleHandler.PollPickerProgress)
	protectedApi.POST("/google/picker/process/:id", googleHandler.ProcessPickerSession)

	// Synology (Protected)
	protectedApi.POST("/synology/test", sh.TestConnection)
	protectedApi.POST("/synology/sync", sh.Sync)
	protectedApi.POST("/synology/clear", sh.Clear)
	protectedApi.GET("/synology/albums", sh.ListAlbums)
	protectedApi.GET("/synology/count", sh.GetPhotoCount)
	protectedApi.POST("/synology/logout", sh.Logout)

	// Google Auth: Login (Protected - User initiates), Callback (Public - Google calls)
	protectedApi.GET("/auth/google/login", googleHandler.Login)
	protectedApi.POST("/auth/google/logout", googleHandler.Logout)

	// Public Callback
	e.GET("/api/auth/google/callback", googleHandler.Callback)

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
	addonPort := os.Getenv("ADDON_PORT")
	if addonPort == "" {
		addonPort = "9607"
	}
	e.Logger.Fatal(e.Start(":" + addonPort))
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
