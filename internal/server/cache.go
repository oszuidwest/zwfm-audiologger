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

// CacheEntry represents a cached segment
// Optimized struct field ordering for Go 1.24+ memory alignment
type CacheEntry struct {
	CreatedAt  time.Time
	AccessedAt time.Time
	FilePath   string
}

// Cache manages cached audio segments
type Cache struct {
	dir     string
	ttl     time.Duration
	entries map[string]*CacheEntry
}

// NewCache creates a new cache instance
func NewCache(dir string, ttl time.Duration) *Cache {
	return &Cache{
		dir:     dir,
		ttl:     ttl,
		entries: make(map[string]*CacheEntry),
	}
}

// Init initializes the cache directory
func (c *Cache) Init() error {
	return os.MkdirAll(c.dir, 0755)
}

// generateCacheKey generates a cache key for a segment request
func (c *Cache) generateCacheKey(streamName string, startTime, endTime time.Time) string {
	data := fmt.Sprintf("%s-%s-%s", streamName, utils.ToAPIString(startTime), utils.ToAPIString(endTime))
	hash := sha256.Sum256([]byte(data))
	return base64.URLEncoding.EncodeToString(hash[:])
}

// GetCachedSegment retrieves a cached segment if it exists and is still valid
func (c *Cache) GetCachedSegment(streamName string, startTime, endTime time.Time) (string, bool) {
	key := c.generateCacheKey(streamName, startTime, endTime)

	entry, exists := c.entries[key]
	if !exists {
		return "", false
	}

	// Check if entry is still valid (TTL)
	if time.Since(entry.CreatedAt) > c.ttl {
		c.removeEntry(key)
		return "", false
	}

	// Check if file still exists
	if _, err := os.Stat(entry.FilePath); os.IsNotExist(err) {
		c.removeEntry(key)
		return "", false
	}

	// Update access time
	entry.AccessedAt = time.Now()

	return entry.FilePath, true
}

// CacheSegment caches a segment file
func (c *Cache) CacheSegment(streamName string, startTime, endTime time.Time, tempFile string) (string, error) {
	key := c.generateCacheKey(streamName, startTime, endTime)

	// Create cached filename (base64 encoded)
	cachedFilename := key + ".mp3"
	cachedPath := filepath.Join(c.dir, cachedFilename)

	// Move temp file to cache
	if err := os.Rename(tempFile, cachedPath); err != nil {
		return "", fmt.Errorf("failed to cache segment: %w", err)
	}

	// Add to cache entries
	c.entries[key] = &CacheEntry{
		FilePath:   cachedPath,
		CreatedAt:  time.Now(),
		AccessedAt: time.Now(),
	}

	return cachedPath, nil
}

// Cleanup removes expired cache entries
func (c *Cache) Cleanup() {
	// Use clear() builtin function from Go 1.21+ for efficient map clearing
	toRemove := make([]string, 0, len(c.entries))

	for key, entry := range c.entries {
		if time.Since(entry.CreatedAt) > c.ttl {
			toRemove = append(toRemove, key)
		}
	}

	for _, key := range toRemove {
		c.removeEntry(key)
	}
}

// removeEntry removes a cache entry and its file
func (c *Cache) removeEntry(key string) {
	if entry, exists := c.entries[key]; exists {
		_ = os.Remove(entry.FilePath) // Ignore error - file might already be deleted
		delete(c.entries, key)
	}
}

// GetCacheStats returns cache statistics
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
		"cache_dir":        c.dir,
		"ttl_hours":        c.ttl.Hours(),
	}
}
