package server

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

type CacheEntry struct {
	CreatedAt  time.Time
	AccessedAt time.Time
	FilePath   string
}

type Cache struct {
	dir     string
	ttl     time.Duration
	entries map[string]*CacheEntry
}

// NewCache returns a new Cache with the specified directory and TTL.
func NewCache(dir string, ttl time.Duration) *Cache {
	return &Cache{
		dir:     dir,
		ttl:     ttl,
		entries: make(map[string]*CacheEntry),
	}
}

// Init creates the cache directory if it doesn't exist.
func (c *Cache) Init() error {
	return os.MkdirAll(c.dir, 0755)
}

// generateCacheKey creates a unique cache key for audio segments
// Format: SHA256(stationName-startTime-endTime) -> base64 URL-safe encoding
// Ensures identical requests for same time range reuse cached segments
func (c *Cache) generateCacheKey(stationName, timezone string, startTime, endTime time.Time) string {
	// Create deterministic string from stream name and time range
	data := fmt.Sprintf("%s-%s-%s", stationName, utils.ToAPIString(startTime, timezone), utils.ToAPIString(endTime, timezone))
	// Hash to fixed-length, collision-resistant key
	hash := sha256.Sum256([]byte(data))
	// URL-safe base64 encoding for filesystem compatibility
	return base64.URLEncoding.EncodeToString(hash[:])
}

// GetCachedSegment retrieves a cached audio segment if valid
// Performs TTL check and file existence validation before returning
func (c *Cache) GetCachedSegment(stationName, timezone string, startTime, endTime time.Time) (string, bool) {
	key := c.generateCacheKey(stationName, timezone, startTime, endTime)

	entry, exists := c.entries[key]
	if !exists {
		return "", false
	}

	// Check if cache entry has expired based on TTL
	if time.Since(entry.CreatedAt) > c.ttl {
		c.removeEntry(key)
		return "", false
	}

	// Verify the cached file still exists on disk
	if _, err := os.Stat(entry.FilePath); os.IsNotExist(err) {
		c.removeEntry(key)
		return "", false
	}

	// Update access time for LRU tracking
	entry.AccessedAt = time.Now()

	return entry.FilePath, true
}

func (c *Cache) CacheSegment(stationName, timezone string, startTime, endTime time.Time, tempFile string) (string, error) {
	key := c.generateCacheKey(stationName, timezone, startTime, endTime)

	cachedFilename := key + ".mp3"
	cachedPath := filepath.Join(c.dir, cachedFilename)

	if err := os.Rename(tempFile, cachedPath); err != nil {
		return "", fmt.Errorf("failed to cache segment: %w", err)
	}

	c.entries[key] = &CacheEntry{
		FilePath:   cachedPath,
		CreatedAt:  time.Now(),
		AccessedAt: time.Now(),
	}

	return cachedPath, nil
}

// Cleanup removes expired cache entries and their associated files
// Two-phase approach: collect expired keys first, then remove them
// This avoids modifying the map while iterating over it
func (c *Cache) Cleanup() {
	toRemove := make([]string, 0, len(c.entries))

	// Phase 1: Identify expired entries
	for key, entry := range c.entries {
		if time.Since(entry.CreatedAt) > c.ttl {
			toRemove = append(toRemove, key)
		}
	}

	// Phase 2: Remove expired entries and their files
	for _, key := range toRemove {
		c.removeEntry(key)
	}
}

func (c *Cache) removeEntry(key string) {
	if entry, exists := c.entries[key]; exists {
		_ = os.Remove(entry.FilePath)
		delete(c.entries, key)
	}
}

func (c *Cache) GetCacheStats() map[string]interface{} {
	var totalSize int64
	validEntries := 0

	for _, entry := range c.entries {
		if time.Since(entry.CreatedAt) <= c.ttl {
			if stat, err := os.Stat(entry.FilePath); err == nil {
				totalSize += stat.Size()
				validEntries++
			}
		}
	}

	return map[string]interface{}{
		"total_entries":    len(c.entries),
		"valid_entries":    validEntries,
		"total_size_bytes": totalSize,
		"cache_directory":  c.dir,
		"ttl_hours":        c.ttl.Hours(),
	}
}
