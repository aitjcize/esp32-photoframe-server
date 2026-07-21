package service

import (
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aitjcize/esp32-photoframe-server/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	immichCacheDirName = "immich_cache"
	maxCacheDimension  = 1600
)

// ImmichCacheService manages the local image cache for Immich assets.
type ImmichCacheService struct {
	db         *gorm.DB
	silentDB   *gorm.DB // db with SQL logging suppressed (cache lookups)
	settings   *SettingsService
	immich     *ImmichService
	dataDir    string
	populateW  sync.WaitGroup
	populating int32
}

// NewImmichCacheService constructs the service.
func NewImmichCacheService(db *gorm.DB, settings *SettingsService, immich *ImmichService, dataDir string) *ImmichCacheService {
	silent := db.Session(&gorm.Session{Logger: db.Logger.LogMode(logger.Silent)})
	return &ImmichCacheService{db: db, silentDB: silent, settings: settings, immich: immich, dataDir: dataDir}
}

// CacheDir returns the absolute path to the cache directory, creating it if needed.
func (s *ImmichCacheService) CacheDir() (string, error) {
	dir := filepath.Join(s.dataDir, immichCacheDirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}
	return dir, nil
}

// Enabled reports whether the Immich image cache is turned on.
func (s *ImmichCacheService) Enabled() bool {
	v, _ := s.settings.Get("immich_cache_enabled")
	return strings.EqualFold(v, "true")
}

// Lookup returns the cached file path for the given image ID, or "" if not cached.
func (s *ImmichCacheService) Lookup(imageID uint) string {
	var row model.ImmichCache
	if err := s.silentDB.Where("image_id = ?", imageID).First(&row).Error; err != nil {
		return "" // cache miss — not an error
	}
	if _, err := os.Stat(row.FilePath); err != nil {
		return ""
	}
	return row.FilePath
}

// CacheImage downloads, resizes, and stores an image in the cache. If the
// image is already cached, it updates the cached_at timestamp. Returns the
// path to the cached file.
func (s *ImmichCacheService) CacheImage(imageID uint, assetID string) (string, error) {
	data, err := s.immich.DownloadPhoto(assetID)
	if err != nil {
		return "", fmt.Errorf("download for cache: %w", err)
	}

	dir, err := s.CacheDir()
	if err != nil {
		return "", err
	}

	cachePath := filepath.Join(dir, assetID+".jpg")

	tmpDir, err := os.MkdirTemp("", "immich-cache-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input")
	outputPath := filepath.Join(tmpDir, "output.jpg")

	if err := os.WriteFile(inputPath, data, 0644); err != nil {
		return "", fmt.Errorf("write temp input: %w", err)
	}

	dimArg := fmt.Sprintf("%dx%d>", maxCacheDimension, maxCacheDimension)
	cmd := exec.Command("magick", inputPath, "-auto-orient", "-resize", dimArg, "-quality", "95", outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ImageMagick resize: %v (output: %s)", err, string(output))
	}

	resized, err := os.ReadFile(outputPath)
	if err != nil {
		return "", fmt.Errorf("read resized output: %w", err)
	}

	img, _, err := image.Decode(strings.NewReader(string(resized)))
	if err != nil {
		// If decode fails, store the raw converted file as-is
		if werr := os.WriteFile(cachePath, resized, 0644); werr != nil {
			return "", fmt.Errorf("write cache file: %w", werr)
		}
	} else {
		f, ferr := os.Create(cachePath)
		if ferr != nil {
			return "", fmt.Errorf("create cache file: %w", ferr)
		}
		if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 95}); err != nil {
			f.Close()
			return "", fmt.Errorf("jpeg encode: %w", err)
		}
		f.Close()
	}

	info, _ := os.Stat(cachePath)
	var size int64
	if info != nil {
		size = info.Size()
	}

	width, height := 0, 0
	if img != nil {
		bounds := img.Bounds()
		width, height = bounds.Dx(), bounds.Dy()
	}

	now := time.Now()
	var existing model.ImmichCache
	err = s.silentDB.Where("image_id = ?", imageID).Take(&existing).Error
	if err == gorm.ErrRecordNotFound {
		row := model.ImmichCache{
			ImageID:   imageID,
			AssetID:   assetID,
			FilePath:  cachePath,
			Width:     width,
			Height:    height,
			SizeBytes: size,
			CachedAt:  now,
		}
		if err := s.db.Create(&row).Error; err != nil {
			return "", fmt.Errorf("insert cache row: %w", err)
		}
	} else {
		s.db.Model(&existing).Updates(map[string]interface{}{
			"file_path":  cachePath,
			"width":      width,
			"height":     height,
			"size_bytes": size,
			"cached_at":  now,
		})
	}

	return cachePath, nil
}

// PopulateNow triggers an async cache population cycle in the background.
func (s *ImmichCacheService) PopulateNow() {
	s.populateW.Add(1)
	go func() {
		defer s.populateW.Done()
		s.populate()
	}()
}

// WaitForPopulation blocks until any in-flight population cycle finishes.
func (s *ImmichCacheService) WaitForPopulation() {
	s.populateW.Wait()
}

// CacheStatus reports the current cache state.
type CacheStatus struct {
	Enabled    bool   `json:"enabled"`
	Count      int64  `json:"count"`
	SizeBytes  int64  `json:"size_bytes"`
	SizeHuman  string `json:"size_human"`
	MaxImages  int    `json:"max_images"`
	MaxSizeMB  int    `json:"max_size_mb"`
	Populating bool   `json:"populating"`
}

// Status returns the current cache status for the UI.
func (s *ImmichCacheService) Status() CacheStatus {
	var count int64
	s.db.Model(&model.ImmichCache{}).Count(&count)
	var totalSize int64
	s.db.Model(&model.ImmichCache{}).Select("COALESCE(SUM(size_bytes),0)").Scan(&totalSize)

	return CacheStatus{
		Enabled:    s.Enabled(),
		Count:      count,
		SizeBytes:  totalSize,
		SizeHuman:  humanSize(totalSize),
		MaxImages:  s.maxImages(),
		MaxSizeMB:  s.maxSizeMB(),
		Populating: s.IsPopulating(),
	}
}

// IsPopulating reports whether a background population cycle is running.
func (s *ImmichCacheService) IsPopulating() bool {
	return atomic.LoadInt32(&s.populating) > 0
}

func (s *ImmichCacheService) maxImages() int {
	v, _ := s.settings.Get("immich_cache_max_images")
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func (s *ImmichCacheService) maxSizeMB() int {
	v, _ := s.settings.Get("immich_cache_max_size_mb")
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func (s *ImmichCacheService) populate() {
	if !s.Enabled() {
		return
	}
	atomic.StoreInt32(&s.populating, 1)
	defer atomic.StoreInt32(&s.populating, 0)
	log.Println("[immich-cache] starting background population cycle")

	weights := s.parseWeights()
	totalWeight := 0
	for _, w := range weights {
		totalWeight += w
	}
	if totalWeight == 0 {
		log.Println("[immich-cache] all weights are zero, nothing to do")
		return
	}

	// Fetch all candidates per bucket (unbounded).
	buckets := s.fetchBuckets(weights)

	// Build set of already-cached image IDs to skip.
	cached := s.cachedImageIDs()
	cachedSet := make(map[uint]struct{}, len(cached))
	for _, id := range cached {
		cachedSet[id] = struct{}{}
	}

	maxImages := s.maxImages()
	maxSizeMB := s.maxSizeMB()

	// Weighted round-robin: pick from each bucket in proportion to its weight.
	type liveBucket struct {
		bucket cacheBucket
		cursor int
	}
	var bs []liveBucket
	for _, b := range buckets {
		if b.weight <= 0 || len(b.images) == 0 {
			continue
		}
		bs = append(bs, liveBucket{bucket: b})
	}

	added := 0
	for len(bs) > 0 {
		// Check image count budget.
		if maxImages > 0 {
			var count int64
			s.silentDB.Model(&model.ImmichCache{}).Count(&count)
			if int(count) >= maxImages {
				break
			}
		}
		// Check size budget.
		if maxSizeMB > 0 {
			var totalSize int64
			s.silentDB.Model(&model.ImmichCache{}).Select("COALESCE(SUM(size_bytes),0)").Scan(&totalSize)
			if totalSize/1024/1024 >= int64(maxSizeMB) {
				break
			}
		}

		// Pick the bucket with the highest accumulated proportion.
		// Each bucket "earns" weight points per round; pick the one with the
		// most unclaimed proportion.
		bestIdx := 0
		bestScore := -1.0
		for i := range bs {
			score := float64(bs[i].bucket.weight) * float64(added+1) / float64(totalWeight)
			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
		}

		// Find next uncached image in this bucket.
		b := &bs[bestIdx]
		found := false
		for b.cursor < len(b.bucket.images) {
			img := b.bucket.images[b.cursor]
			b.cursor++
			if _, ok := cachedSet[img.ID]; ok {
				continue
			}
			cachedSet[img.ID] = struct{}{}
			if _, err := s.CacheImage(img.ID, img.ExternalID); err != nil {
				log.Printf("[immich-cache] failed to cache image %d (%s): %v", img.ID, img.ExternalID, err)
			}
			added++
			found = true
			break
		}
		if !found {
			// Bucket exhausted — remove it.
			bs = append(bs[:bestIdx], bs[bestIdx+1:]...)
		}
	}

	s.prune()
	s.gcOrphanCacheFiles()
	log.Printf("[immich-cache] population cycle complete: added %d images", added)
}

type cacheBucket struct {
	images []model.Image
	weight int
}

// fetchBuckets loads all candidates per weight bucket.
func (s *ImmichCacheService) fetchBuckets(weights map[string]int) []cacheBucket {
	var buckets []cacheBucket

	dateFrom, dateTo := s.immich.DateRange()
	dateCond, dateArgs := s.buildDateRangeClause(dateFrom, dateTo)

	if w, ok := weights["favorites"]; ok && w > 0 {
		var imgs []model.Image
		query := `SELECT DISTINCT i.* FROM images i
			JOIN image_album_memberships iam ON iam.image_id = i.id
			JOIN albums a ON a.id = iam.album_id
			WHERE i.source = ? AND a.external_id = ? AND i.deleted_at IS NULL` + dateCond
		args := append([]interface{}{model.SourceImmich, model.ImmichVirtualFavorites}, dateArgs...)
		s.db.Raw(query, args...).Scan(&imgs)
		buckets = append(buckets, cacheBucket{images: imgs, weight: w})
	}
	if w, ok := weights["recent"]; ok && w > 0 {
		var imgs []model.Image
		query := `SELECT * FROM images
			WHERE source = ? AND photo_taken_at > datetime('now', '-30 days') AND deleted_at IS NULL` + dateCond
		args := append([]interface{}{model.SourceImmich}, dateArgs...)
		s.db.Raw(query, args...).Scan(&imgs)
		buckets = append(buckets, cacheBucket{images: imgs, weight: w})
	}
	if w, ok := weights["random"]; ok && w > 0 {
		var imgs []model.Image
		query := `SELECT * FROM images WHERE source = ? AND deleted_at IS NULL` + dateCond + ` ORDER BY RANDOM()`
		args := append([]interface{}{model.SourceImmich}, dateArgs...)
		s.db.Raw(query, args...).Scan(&imgs)
		buckets = append(buckets, cacheBucket{images: imgs, weight: w})
	}
	if w, ok := weights["old"]; ok && w > 0 {
		var imgs []model.Image
		query := `SELECT * FROM images
			WHERE source = ? AND (photo_taken_at IS NULL OR photo_taken_at <= datetime('now', '-30 days'))
			AND deleted_at IS NULL` + dateCond + ` ORDER BY RANDOM()`
		args := append([]interface{}{model.SourceImmich}, dateArgs...)
		s.db.Raw(query, args...).Scan(&imgs)
		buckets = append(buckets, cacheBucket{images: imgs, weight: w})
	}

	return buckets
}

// buildDateRangeClause returns a SQL WHERE fragment and args for the date range.
func (s *ImmichCacheService) buildDateRangeClause(from, to time.Time) (string, []interface{}) {
	var conds []string
	var args []interface{}
	if !from.IsZero() {
		conds = append(conds, " AND photo_taken_at >= ?")
		args = append(args, from)
	}
	if !to.IsZero() {
		conds = append(conds, " AND photo_taken_at <= ?")
		args = append(args, to)
	}
	return strings.Join(conds, ""), args
}

func (s *ImmichCacheService) parseWeights() map[string]int {
	defaultWeights := "favorites=50,recent=30,random=20"
	v, _ := s.settings.Get("immich_cache_priority")
	if v == "" {
		v = defaultWeights
	}
	out := map[string]int{}
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val, err := strconv.Atoi(strings.TrimSpace(kv[1]))
		if err != nil {
			continue
		}
		out[key] = val
	}
	return out
}

func (s *ImmichCacheService) cachedImageIDs() []uint {
	var ids []uint
	s.db.Model(&model.ImmichCache{}).Pluck("image_id", &ids)
	return ids
}

// prune evicts oldest cache entries until budget is satisfied.
func (s *ImmichCacheService) prune() {
	maxImages := s.maxImages()
	maxSizeMB := s.maxSizeMB()
	if maxImages <= 0 && maxSizeMB <= 0 {
		return
	}

	for {
		var count int64
		s.db.Model(&model.ImmichCache{}).Count(&count)
		var totalSize int64
		s.db.Model(&model.ImmichCache{}).Select("COALESCE(SUM(size_bytes),0)").Scan(&totalSize)

		overCount := maxImages > 0 && int(count) > maxImages
		overSize := maxSizeMB > 0 && totalSize/1024/1024 > int64(maxSizeMB)
		if !overCount && !overSize {
			break
		}

		var oldest model.ImmichCache
		if err := s.silentDB.Order("cached_at ASC").First(&oldest).Error; err != nil {
			break
		}
		os.Remove(oldest.FilePath)
		s.db.Unscoped().Delete(&model.ImmichCache{}, oldest.ID)
	}
}

// gcOrphanCacheFiles removes cache entries whose image row has been deleted.
func (s *ImmichCacheService) gcOrphanCacheFiles() {
	var entries []model.ImmichCache
	s.db.Find(&entries)
	for _, entry := range entries {
		var exists int64
		s.db.Model(&model.Image{}).Where("id = ?", entry.ImageID).Count(&exists)
		if exists == 0 {
			os.Remove(entry.FilePath)
			s.db.Unscoped().Delete(&model.ImmichCache{}, entry.ID)
		}
	}
}

// ClearCache deletes all cached images from disk and the database.
func (s *ImmichCacheService) ClearCache() error {
	var entries []model.ImmichCache
	if err := s.db.Find(&entries).Error; err != nil {
		return fmt.Errorf("list cache entries: %w", err)
	}
	for _, entry := range entries {
		os.Remove(entry.FilePath)
	}
	if err := s.db.Unscoped().Where("1 = 1").Delete(&model.ImmichCache{}).Error; err != nil {
		return fmt.Errorf("clear cache table: %w", err)
	}
	log.Printf("[immich-cache] cleared %d cached images", len(entries))
	return nil
}

// humanSize returns a human-readable size string.
func humanSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
