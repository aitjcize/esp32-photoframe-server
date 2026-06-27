package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/db"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/handler"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/imagesource"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/middleware"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"github.com/aitjcize/esp32-photoframe-server/backend/internal/service"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/gcalendar"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/googlephotos"
	"github.com/aitjcize/esp32-photoframe-server/backend/pkg/weather"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
	"gorm.io/gorm"
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

	// Backfill existing API keys with device IDs (matches token Name to Device Name)
	backfillDeviceTokens(database)

	// Initialize Services
	settingsService := service.NewSettingsService(database)
	tokenStore := service.NewDBTokenStore(database, "photos")
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

	// Initialize Google Photos Client
	// Pass settingsService as ConfigProvider so it fetches latest config on every request
	googleClient := googlephotos.NewClient(settingsService, tokenStore)

	// Initialize Google Calendar Client (separate OAuth, separate token)
	calendarTokenStore := service.NewDBTokenStore(database, "calendar")
	calendarConfigProvider := service.NewCalendarConfigProvider(settingsService)
	googleCalendarClient := googlephotos.NewClient(calendarConfigProvider, calendarTokenStore)

	// Initialize Processor
	processorService := service.NewProcessorService()
	weatherClient := weather.NewClient()
	calendarClient := gcalendar.NewClient()
	// Initialize Renderer (HTML/CSS → image via headless Chrome)
	// Chrome is launched lazily on first render request to save memory.
	rendererService, err := service.NewRendererService()
	if err != nil {
		log.Fatalf("Failed to initialize renderer: %v", err)
	}
	// Initialize Synology Photos Service
	synologyService := service.NewSynologyService(database, settingsService)
	synologyService.StartAutoSync()
	// Initialize Immich Service
	immichService := service.NewImmichService(database, settingsService)
	immichService.StartAutoSync()
	// Initialize AI Generation Service
	aiGenerationService := service.NewAIGenerationService(settingsService)
	// Initialize Generative (procedural) Service — fractal / DLA / …
	generativeService := service.NewGenerativeService(database)

	// Image-source registry: every image source — synthetic and library —
	// registers here as its own plugin. The order below mirrors the order
	// of the source dropdown in webapp/src/components/Settings.vue so the
	// two stay easy to scan against each other.
	sourceRegistry := imagesource.NewRegistry()
	sourceRegistry.Register(service.NewGallerySource(database, dataDir))
	sourceRegistry.Register(service.NewImmichSource(database, immichService))
	sourceRegistry.Register(service.NewGooglePhotosSource(database, dataDir))
	sourceRegistry.Register(service.NewSynologyPhotosSource(database, synologyService))
	sourceRegistry.Register(service.NewURLProxySource(database))
	sourceRegistry.Register(service.NewAIGenerationSource(aiGenerationService))
	sourceRegistry.Register(service.NewFractalSource(generativeService))
	sourceRegistry.Register(service.NewDLASource(generativeService))

	// Initialize Picker Service
	// dataDir already set from migration logic above
	// Ensure dataDir exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	cleanupTempThumbnails(dataDir)

	pickerService := service.NewPickerService(googleClient, database, dataDir)

	// Initialize Device Service
	deviceService := service.NewDeviceService(service.DeviceServiceDeps{
		DB:             database,
		Settings:       settingsService,
		Processor:      processorService,
		Renderer:       rendererService,
		Weather:        weatherClient,
		Calendar:       calendarClient,
		CalendarGoogle: googleCalendarClient,
	})
	deviceHandler := handler.NewDeviceHandler(deviceService, synologyService, immichService, database)

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
	h := handler.NewHandler(settingsService, telegramService, googleClient, googleCalendarClient)
	googleHandler := handler.NewGoogleHandler(googleClient, googleCalendarClient, pickerService, database, dataDir)
	sh := handler.NewSynologyHandler(synologyService)
	imh := handler.NewImmichHandler(immichService)
	// Generic photo-sync endpoints (sync / sync-status / clear / count) shared
	// across sources.
	synologySync := handler.NewPhotoSyncHandler(synologyService)
	immichSync := handler.NewPhotoSyncHandler(immichService)
	gh := handler.NewGalleryHandler(database, synologyService, immichService, dataDir)
	ih := handler.NewImageHandler(handler.ImageHandlerDeps{
		Settings:       settingsService,
		Renderer:       rendererService,
		Processor:      processorService,
		CalendarGoogle: googleCalendarClient,
		Sources:        sourceRegistry,
		Weather:        weatherClient,
		Calendar:       calendarClient,
		Auth:           authService,
		DB:             database,
		DataDir:        dataDir,
	})
	ch := handler.NewCalendarHandler(googleCalendarClient, calendarClient)
	ah := handler.NewAuthHandler(authService)

	// Echo instance
	e := echo.New()

	// Middleware. Log ${path} (not ${uri}) so secrets in query strings — e.g.
	// the device ?token= — never end up in access logs.
	e.Use(echoMiddleware.LoggerWithConfig(echoMiddleware.LoggerConfig{
		Format: `{"time":"${time_rfc3339_nano}","remote_ip":"${remote_ip}",` +
			`"method":"${method}","path":"${path}","status":${status},` +
			`"error":"${error}","latency_human":"${latency_human}"}` + "\n",
	}))
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
	// Unified endpoint: source resolved server-side from the device.
	e.GET("/image", ih.ServeImage, authMiddleware)

	// Thumbnail likely needs protection too, or obscure IDs. For now, keep public as they are temporary?
	// User said "access the /image/<source>/ endpoint. This one... people can't just access".
	// Let's protect main image endpoint.
	e.GET("/served-image-thumbnail/:id", ih.GetServedImageThumbnail)

	// Device Config Sync (Protected - device token or session auth)
	e.POST("/api/device-config/sync", ih.SyncDeviceConfig, authMiddleware)

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
	protectedApi.POST("/devices/:id/refresh", deviceHandler.RefreshDevice)
	protectedApi.GET("/devices/:id/config", ih.GetDeviceConfig)
	protectedApi.PUT("/devices/:id/config", ih.UpdateDeviceConfig)
	protectedApi.GET("/devices/:id/albums", deviceHandler.GetDeviceAlbums)
	protectedApi.PUT("/devices/:id/albums", deviceHandler.UpdateDeviceAlbums)

	// Source albums (persisted), shared by device picker + gallery
	protectedApi.GET("/albums", deviceHandler.ListAlbums)

	// Device Tokens (Protected)
	protectedApi.POST("/auth/tokens", ah.GenerateDeviceToken)
	protectedApi.GET("/auth/tokens", ah.ListTokens)
	protectedApi.PUT("/auth/tokens/:id", ah.UpdateTokenDevice)
	protectedApi.DELETE("/auth/tokens/:id", ah.RevokeToken)
	protectedApi.GET("/auth/sessions", ah.ListSessions)
	protectedApi.DELETE("/auth/sessions/:id", ah.RevokeSession)
	protectedApi.POST("/auth/account", ah.UpdateAccount)

	// Gallery (Protected) - Unified
	protectedApi.GET("/gallery/photos", gh.ListPhotos)
	protectedApi.POST("/gallery/upload", gh.UploadPhoto)
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
	protectedApi.POST("/synology/sync", synologySync.Sync)
	protectedApi.GET("/synology/sync-status", synologySync.SyncStatus)
	protectedApi.POST("/synology/clear", synologySync.Clear)
	protectedApi.GET("/synology/albums", sh.ListAlbums)
	protectedApi.POST("/synology/sync-albums", sh.SetSyncAlbums)
	protectedApi.GET("/synology/count", synologySync.Count)
	protectedApi.POST("/synology/logout", sh.Logout)

	// Immich (Protected)
	protectedApi.POST("/immich/test", imh.TestConnection)
	protectedApi.POST("/immich/sync", immichSync.Sync)
	protectedApi.GET("/immich/sync-status", immichSync.SyncStatus)
	protectedApi.POST("/immich/clear", immichSync.Clear)
	protectedApi.GET("/immich/albums", imh.ListAlbums)
	protectedApi.POST("/immich/sync-albums", imh.SetSyncAlbums)
	protectedApi.GET("/immich/count", immichSync.Count)

	// Calendar (Protected)
	protectedApi.GET("/calendar/calendars", ch.ListCalendars)

	// Google Auth (Photos + Calendar share the same callback via state parameter)
	protectedApi.GET("/auth/google/login", googleHandler.Login)
	protectedApi.POST("/auth/google/logout", googleHandler.Logout)
	protectedApi.GET("/auth/google-calendar/login", googleHandler.CalendarLogin)
	protectedApi.POST("/auth/google-calendar/logout", googleHandler.CalendarLogout)

	// Public Callback (shared by both Photos and Calendar)
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

	// 3. Serve favicon from the root of the static dir
	e.File("/favicon.svg", filepath.Join(staticDir, "favicon.svg"))

	// 4. SPA Fallback: Any other route not matched (api is already handled) goes to index.html
	e.GET("/*", func(c echo.Context) error {
		return c.File(filepath.Join(staticDir, "index.html"))
	})

	// Start server
	listenPort := os.Getenv("PORT")
	if listenPort == "" {
		listenPort = "9607"
	}
	e.Logger.Fatal(e.Start(":" + listenPort))
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

// backfillDeviceTokens associates existing API keys with devices by matching
// the token name to device names. This is idempotent and handles the migration
// from tokens without device_id to tokens with device_id.
// Ambiguous matches (multiple devices with the same name) are skipped.
func backfillDeviceTokens(database *gorm.DB) {
	var keys []model.APIKey
	database.Where("device_id IS NULL").Find(&keys)
	for _, key := range keys {
		var devices []model.Device
		database.Where("name = ?", key.Name).Find(&devices)
		if len(devices) == 1 {
			database.Model(&key).Update("device_id", devices[0].ID)
			log.Printf("Backfilled API key %d (%s) with device ID %d", key.ID, key.Name, devices[0].ID)
		} else if len(devices) > 1 {
			log.Printf("Skipping backfill for API key %d (%s): %d devices with the same name", key.ID, key.Name, len(devices))
		}
	}
}
