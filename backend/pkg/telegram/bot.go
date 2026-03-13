package telegram

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	_ "image/jpeg"
	_ "image/png"
	"image"
	tele "gopkg.in/telebot.v3"
	"gorm.io/gorm"
)

type SettingsProvider interface {
	Get(key string) (string, error)
	Set(key string, value string) error
}

type Pusher interface {
	PushToHost(device *model.Device, imagePath string, extraOpts map[string]string) error
}

type Bot struct {
	b        *tele.Bot
	db       *gorm.DB
	dataDir  string
	settings SettingsProvider
	pusher   Pusher
}

func NewBot(token string, db *gorm.DB, dataDir string, settings SettingsProvider, pusher Pusher) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}

	bot := &Bot{
		b:        b,
		db:       db,
		dataDir:  dataDir,
		settings: settings,
		pusher:   pusher,
	}
	bot.registerHandlers()

	return bot, nil
}

func (bot *Bot) Start() {
	log.Println("Telegram bot started")
	// Set command menu
	commands := []tele.Command{
		{Text: "start", Description: "Start the bot"},
		{Text: "help", Description: "Show available commands"},
		{Text: "list", Description: "List photos (e.g. /list 2)"},
		{Text: "get", Description: "Retrieve photos by ID (e.g. /get 1 2 3)"},
		{Text: "delete", Description: "Delete photos by ID (e.g. /delete 1 2 3)"},
		{Text: "next", Description: "Force next photo by ID (e.g. /next 1)"},
		{Text: "clear", Description: "Clear all Telegram photos"},
	}
	if err := bot.b.SetCommands(commands); err != nil {
		log.Printf("Failed to set bot commands: %v", err)
	}
	go bot.b.Start()
}

func (bot *Bot) Stop() {
	bot.b.Stop()
}

func (bot *Bot) registerHandlers() {
	bot.b.Handle("/start", func(c tele.Context) error {
		authUsers, _ := bot.settings.Get("telegram_auth_users")
		if authUsers == "" {
			// Whitelist is empty, automatically add this first user
			userID := fmt.Sprintf("%d", c.Sender().ID)
			if err := bot.settings.Set("telegram_auth_users", userID); err != nil {
				log.Printf("Failed to auto-whitelist first user: %v", err)
				return c.Send("Hello! I could not automatically add you to the whitelist. Please configure it manually in the dashboard.")
			}
			return c.Send(fmt.Sprintf("Hello! You are the first user to start this bot, so I've automatically added your ID (%s) to the whitelist.\n\nYou can now send me photos to display on your frame! Use /help to see all commands.", userID))
		}

		if !bot.isAuthorized(c.Sender().ID) {
			return c.Send("Hello! This bot is currently locked to specific users. Please contact the administrator to be whitelisted.")
		}

		return c.Send("Hello back! You are already whitelisted. Send me a photo to display on your frame or use /help to see available commands.")
	})

	bot.b.Handle("/help", bot.handleHelp)
	bot.b.Handle("/list", bot.handleList)
	bot.b.Handle("/get", bot.handleGet)
	bot.b.Handle("/delete", bot.handleDelete)
	bot.b.Handle("/next", bot.handleNext)
	bot.b.Handle("/clear", bot.handleClear)

	bot.b.Handle(tele.OnPhoto, bot.handlePhoto)
	bot.b.Handle(tele.OnText, bot.handleText)

	// Handle button clicks
	bot.b.Handle(tele.OnCallback, func(c tele.Context) error {
		data := c.Callback().Data
		if strings.HasPrefix(data, "delete_") {
			idStr := strings.TrimPrefix(data, "delete_")
			id, _ := strconv.Atoi(idStr)
			if err := bot.deleteImage(uint(id)); err != nil {
				return c.Respond(&tele.CallbackResponse{Text: "Failed to delete: " + err.Error()})
			}
			return c.Respond(&tele.CallbackResponse{Text: "Deleted!"})
		}
		return nil
	})
}

func (bot *Bot) handleHelp(c tele.Context) error {
	helpText := `Available Commands:
/start - Start the bot
/help - Show this help message
/list [page] - List photos (10 per page, e.g. /list 2)
/next <id> - Force the next photo to be shown by ID
/get <id1> <id2>... - Retrieve photos from the server
/delete <id1> <id2>... - Delete photos from the server
/clear - Remove all Telegram photos

You can also send me any photo to add it to the rotation. You can include a message with the photo to "name" it, which makes it easier to find and manage when using the /list command. Every photo you send will have a "Delete" button under it for easy removal.`
	return c.Send(helpText)
}

func (bot *Bot) isAuthorized(userID int64) bool {
	authUsers, _ := bot.settings.Get("telegram_auth_users")
	if authUsers == "" {
		return true // Whitelist disabled
	}
	for _, idStr := range strings.Split(authUsers, ",") {
		if strings.TrimSpace(idStr) == fmt.Sprintf("%d", userID) {
			return true
		}
	}
	return false
}

func (bot *Bot) handleList(c tele.Context) error {
	if !bot.isAuthorized(c.Sender().ID) {
		return c.Send("Unauthorized.")
	}

	page := 1
	pageSize := 10
	args := c.Args()
	if len(args) > 0 {
		if p, err := strconv.Atoi(args[0]); err == nil && p > 0 {
			page = p
		}
	}

	var total int64
	bot.db.Model(&model.Image{}).Where("source = ?", model.SourceTelegram).Count(&total)

	var images []model.Image
	offset := (page - 1) * pageSize
	if err := bot.db.Where("source = ?", model.SourceTelegram).Order("created_at desc").Offset(offset).Limit(pageSize).Find(&images).Error; err != nil {
		return c.Send("Failed to list images.")
	}

	if len(images) == 0 {
		return c.Send("No images found on this page.")
	}

	var resp strings.Builder
	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))
	resp.WriteString(fmt.Sprintf("Telegram photos (Page %d/%d):\n\n", page, totalPages))
	
	for _, img := range images {
		resp.WriteString(fmt.Sprintf("ID: %d - %s (%s)\n", img.ID, img.CreatedAt.Format("Jan 02 15:04"), img.Caption))
	}
	
	resp.WriteString("\nCommands:\n")
	resp.WriteString("• `/get <id1> <id2>...` to retrieve\n")
	resp.WriteString("• `/delete <id1> <id2>...` to remove\n")
	if page < totalPages {
		resp.WriteString(fmt.Sprintf("• `/list %d` for next page", page+1))
	} else if page > 1 {
		resp.WriteString("• `/list 1` back to first page")
	}

	return c.Send(resp.String())
}

func (bot *Bot) handleGet(c tele.Context) error {
	if !bot.isAuthorized(c.Sender().ID) {
		return c.Send("Unauthorized.")
	}

	args := c.Args()
	if len(args) == 0 {
		return c.Send("Usage: /get <id> [<id>...]")
	}

	for _, arg := range args {
		id, err := strconv.Atoi(arg)
		if err != nil {
			c.Send(fmt.Sprintf("Invalid ID: %s", arg))
			continue
		}

		var img model.Image
		if err := bot.db.Where("id = ? AND source = ?", uint(id), model.SourceTelegram).First(&img).Error; err != nil {
			c.Send(fmt.Sprintf("Image %d not found.", id))
			continue
		}

		photo := &tele.Photo{
			File:    tele.FromDisk(img.FilePath),
			Caption: fmt.Sprintf("ID: %d\n%s", img.ID, img.Caption),
		}

		if msgSent, err := bot.b.Send(c.Recipient(), photo); err == nil {
			bot.db.Model(&img).Update("telegram_bot_message_id", msgSent.ID)
		} else {
			log.Printf("Failed to send photo %d: %v", id, err)
			c.Send(fmt.Sprintf("Failed to send image %d.", id))
		}
	}

	return nil
}

func (bot *Bot) handleDelete(c tele.Context) error {
	if !bot.isAuthorized(c.Sender().ID) {
		return c.Send("Unauthorized.")
	}

	args := c.Args()
	if len(args) == 0 {
		return c.Send("Usage: /delete <id1> <id2> ...")
	}

	var successCount int
	var failMsgs []string

	for _, arg := range args {
		id, err := strconv.Atoi(arg)
		if err != nil {
			failMsgs = append(failMsgs, fmt.Sprintf("Invalid ID: %s", arg))
			continue
		}

		if err := bot.deleteImage(uint(id)); err != nil {
			failMsgs = append(failMsgs, fmt.Sprintf("ID %d: %v", id, err))
		} else {
			successCount++
		}
	}

	resp := fmt.Sprintf("Deleted %d images.", successCount)
	if len(failMsgs) > 0 {
		resp += "\nFailed:\n" + strings.Join(failMsgs, "\n")
	}

	return c.Send(resp)
}

func (bot *Bot) handleNext(c tele.Context) error {
	if !bot.isAuthorized(c.Sender().ID) {
		return c.Send("Unauthorized.")
	}

	args := c.Args()
	if len(args) == 0 {
		return c.Send("Usage: /next <id>")
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		return c.Send("Invalid ID format.")
	}

	// Verify image exists and is Telegram source
	var img model.Image
	if err := bot.db.Where("id = ? AND source = ?", uint(id), model.SourceTelegram).First(&img).Error; err != nil {
		return c.Send(fmt.Sprintf("Image ID %d not found in Telegram collection.", id))
	}

	// Set as global fallback
	bot.settings.Set("telegram_pushed_image_id", fmt.Sprintf("%d", id))

	// Get target devices
	targetDeviceIDStr, _ := bot.settings.Get("telegram_target_device_id")
	if targetDeviceIDStr != "" {
		targetIDs := strings.Split(targetDeviceIDStr, ",")
		for _, devIDStr := range targetIDs {
			devIDStr = strings.TrimSpace(devIDStr)
			if devIDStr == "" {
				continue
			}
			devID, _ := strconv.Atoi(devIDStr)
			bot.db.Model(&model.Device{}).Where("id = ?", uint(devID)).Update("pushed_image_id", uint(id))
		}
	}

	return c.Send(fmt.Sprintf("Next image set to ID %d. It will be shown on the next request.", id))
}

func (bot *Bot) deleteImage(id uint) error {
	var img model.Image
	if err := bot.db.Where("id = ? AND source = ?", id, model.SourceTelegram).First(&img).Error; err != nil {
		return err
	}

	// Delete file
	if err := os.Remove(img.FilePath); err != nil && !os.IsNotExist(err) {
		log.Printf("Failed to delete file %s: %v", img.FilePath, err)
	}

	// Delete record
	return bot.db.Unscoped().Delete(&img).Error
}

func (bot *Bot) handleClear(c tele.Context) error {
	if !bot.isAuthorized(c.Sender().ID) {
		return c.Send("Unauthorized.")
	}

	var images []model.Image
	bot.db.Where("source = ?", model.SourceTelegram).Find(&images)

	for _, img := range images {
		os.Remove(img.FilePath)
		bot.db.Unscoped().Delete(&img)
	}

	return c.Send(fmt.Sprintf("Cleared %d images.", len(images)))
}

func (bot *Bot) handlePhoto(c tele.Context) error {
	if !bot.isAuthorized(c.Sender().ID) {
		return c.Send("Unauthorized.")
	}

	// Download photo
	photo := c.Message().Photo

	// Create directory if not exists
	photosDir := filepath.Join(bot.dataDir, "photos")
	if err := os.MkdirAll(photosDir, 0755); err != nil {
		return c.Send("Failed to create photos directory.")
	}

	// Target file path (unique)
	filename := fmt.Sprintf("telegram_%d_%d.jpg", c.Message().ID, time.Now().Unix())
	destPath := filepath.Join(photosDir, filename)

	// Download
	if err := bot.b.Download(&photo.File, destPath); err != nil {
		return c.Send("Failed to download photo: " + err.Error())
	}

	// Overwrite last photo for backward compatibility if needed, 
	// or just let the new logic take over. Let's overwrite telegram_last.jpg too
	// so existing configurations don't break immediately.
	lastPath := filepath.Join(photosDir, "telegram_last.jpg")
	if data, err := os.ReadFile(destPath); err == nil {
		os.WriteFile(lastPath, data, 0644)
	}

	// Get Image context for DB
	width, height := 0, 0
	orientation := "landscape"
	if f, err := os.Open(destPath); err == nil {
		if img, _, err := image.DecodeConfig(f); err == nil {
			width = img.Width
			height = img.Height
			if height > width {
				orientation = "portrait"
			}
		}
		f.Close()
	}

	// Create DB Record
	caption := c.Message().Caption
	img := model.Image{
		FilePath:          destPath,
		Caption:           caption,
		Source:            model.SourceTelegram,
		Width:             width,
		Height:            height,
		Orientation:       orientation,
		TelegramMessageID: c.Message().ID,
	}

	if err := bot.db.Create(&img).Error; err != nil {
		log.Printf("Failed to save telegram image to DB: %v", err)
		return c.Send("Photo saved to disk but failed to register in database.")
	}

	// Update Caption Setting ( legacy )
	var setting model.Setting
	setting.Key = "telegram_caption"
	setting.Value = caption
	bot.db.Save(&setting)

	// Inline Keyboard for Management
	menu := &tele.ReplyMarkup{}
	btnDelete := menu.Data("🗑️ Delete", "delete", fmt.Sprintf("%d", img.ID))
	menu.Inline(menu.Row(btnDelete))

	// Check if Push to Device is enabled
	pushEnabled, _ := bot.settings.Get("telegram_push_enabled")
	targetDeviceIDStr, _ := bot.settings.Get("telegram_target_device_id")

	if pushEnabled == "true" && targetDeviceIDStr != "" {
		// Send initial status
		statusMsg, err := bot.b.Send(c.Recipient(), "Connecting to devices...", menu)
		if err != nil {
			log.Printf("Failed to send status message: %v", err)
			return err
		}
		bot.db.Model(&img).Update("telegram_bot_message_id", statusMsg.ID)

		targetIDs := strings.Split(targetDeviceIDStr, ",")
		var successDevices []string
		var failDevices []string

		for _, id := range targetIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}

			// Look up device
			var device model.Device
			if err := bot.db.First(&device, id).Error; err != nil {
				log.Printf("Failed to find target device (ID: %s): %v", id, err)
				failDevices = append(failDevices, fmt.Sprintf("ID %s", id))
				continue
			}

			err = bot.pusher.PushToHost(&device, destPath, nil)
			if err != nil {
				log.Printf("Failed to push to device %s: %v", device.Name, err)
				failDevices = append(failDevices, device.Name)
			} else {
				successDevices = append(successDevices, device.Name)
			}
		}

		var summary strings.Builder
		summary.WriteString(fmt.Sprintf("Photo saved (ID: %d)!\n", img.ID))

		if len(successDevices) > 0 {
			for _, name := range successDevices {
				summary.WriteString(fmt.Sprintf("✅ %s\n", name))
			}
		}

		if len(failDevices) > 0 {
			for _, name := range failDevices {
				summary.WriteString(fmt.Sprintf("❌ %s (Offline/Failed)\n", name))
			}
		}

		msg := summary.String()

		_, editErr := bot.b.Edit(statusMsg, msg, menu)
		if editErr != nil {
			return c.Send(msg, menu)
		}
		return nil
	}

	msg := fmt.Sprintf("Photo updated (ID: %d)! It will show up next time the device awakes.", img.ID)
	if msgSent, err := bot.b.Send(c.Recipient(), msg, menu); err == nil {
		bot.db.Model(&img).Update("telegram_bot_message_id", msgSent.ID)
	}
	return nil
}

func (bot *Bot) handleText(c tele.Context) error {
	if !bot.isAuthorized(c.Sender().ID) {
		return nil
	}

	replyTo := c.Message().ReplyTo
	if replyTo == nil {
		return nil
	}

	// Find the image associated with the bot message being replied to
	var img model.Image
	if err := bot.db.Where("telegram_bot_message_id = ?", replyTo.ID).First(&img).Error; err != nil {
		return nil // Not a reply to a trackable image message
	}

	newCaption := c.Text()
	img.Caption = newCaption
	if err := bot.db.Save(&img).Error; err != nil {
		log.Printf("Failed to update caption for image %d: %v", img.ID, err)
		return c.Send("Failed to update caption.")
	}

	// Update legacy setting if it was the last one (optional, keeping for compatibility)
	var setting model.Setting
	setting.Key = "telegram_caption"
	setting.Value = newCaption
	bot.db.Save(&setting)

	return c.Send(fmt.Sprintf("Caption for image %d updated to: %s", img.ID, newCaption))
}
