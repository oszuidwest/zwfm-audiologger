// Package server provides HTTP server functionality with Gin framework for serving
// audio recordings, metadata, and API endpoints with caching and middleware support.
package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-contrib/cache/persistence"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/oszuidwest/zwfm-audiologger/internal/audio"
	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/logger"
	"github.com/oszuidwest/zwfm-audiologger/internal/metadata"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
	"github.com/oszuidwest/zwfm-audiologger/internal/version"
	ffmpeglib "github.com/u2takey/ffmpeg-go"
)

// AudioClipRequest represents the request parameters for generating audio clips
// with start and end time specifications.
type AudioClipRequest struct {
	Start string `form:"start" json:"start" binding:"required"`
	End   string `form:"end" json:"end" binding:"required"`
}

// RecordingInfo represents basic recording file information including timestamp,
// size, and format details.
type RecordingInfo struct {
	Timestamp string
	Size      int64
	Format    audio.Format
}

// Global variables
var startTime = time.Now()

// GinServer represents the Gin-based HTTP server with configuration, logging,
// and caching capabilities.
type GinServer struct {
	config   *config.Config
	logger   *logger.Logger
	metadata *metadata.Fetcher
	ginCache persistence.CacheStore
	engine   *gin.Engine
}

// NewGinServer creates a new Gin-based server
func NewGinServer(cfg *config.Config, log *logger.Logger) *GinServer {
	// Set Gin mode based on debug setting
	if !cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize in-memory cache store
	ginCache := persistence.NewInMemoryStore(30 * time.Minute)

	return &GinServer{
		config:   cfg,
		logger:   log,
		metadata: metadata.New(log),
		ginCache: ginCache,
		engine:   gin.New(),
	}
}

// Start initializes and starts the Gin HTTP server
func (s *GinServer) Start(ctx context.Context, port string) error {
	if err := os.MkdirAll(s.config.Server.CacheDirectory, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", s.config.Server.CacheDirectory, err)
	}

	// Clean up any leftover temp files from previous runs
	s.cleanupTempFiles()

	s.setupRoutes()
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      s.engine,
		ReadTimeout:  time.Duration(s.config.Server.ReadTimeout),
		WriteTimeout: time.Duration(s.config.Server.WriteTimeout),
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(s.config.Server.ShutdownTimeout))
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

// setupRoutes configures all routes and middleware
func (s *GinServer) setupRoutes() {
	s.engine.Use(gin.Logger())
	s.engine.Use(gin.Recovery())
	s.engine.Use(s.customLoggingMiddleware())
	s.engine.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Health endpoints
	s.engine.GET("/health", s.healthHandler)
	s.engine.GET("/ready", s.readinessHandler)

	// API v1 routes
	v1 := s.engine.Group("/api/v1")
	{
		v1.GET("/system/stats", s.systemStatsHandler)

		// Station endpoints
		v1.GET("/stations", s.stationsHandler)
		v1.GET("/stations/:station", s.stationDetailsHandler)

		// Recording endpoints
		v1.GET("/stations/:station/recordings", s.recordingsHandler)
		v1.GET("/stations/:station/recordings/:timestamp", s.recordingHandler)
		v1.GET("/stations/:station/recordings/:timestamp/play", s.playRecordingHandler)
		v1.GET("/stations/:station/recordings/:timestamp/download", s.downloadRecordingHandler)
		v1.GET("/stations/:station/recordings/:timestamp/metadata", s.metadataHandler)

		// Audio clips (cached for 7 days)
		v1.GET("/stations/:station/clips", s.normalizedCacheMiddleware(7*24*time.Hour), s.audioClipHandler)
	}
}

// normalizedCacheMiddleware creates a cache middleware that normalizes URL-encoded query parameters
// to ensure consistent cache keys for equivalent requests (e.g., encoded vs unencoded colons)
func (s *GinServer) normalizedCacheMiddleware(duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate normalized cache key
		cacheKey := s.generateNormalizedCacheKey(c)

		s.logger.Debug("cache middleware",
			"original_url", c.Request.URL.String(),
			"cache_key", cacheKey,
		)

		// Check cache first
		var cachedData []byte
		if err := s.ginCache.Get(cacheKey, &cachedData); err == nil {
			s.logger.Debug("cache hit", "cache_key", cacheKey)

			// Serve cached response
			c.Header("X-Cache", "HIT")
			c.Data(http.StatusOK, "audio/mpeg", cachedData)
			c.Abort()
			return
		}

		s.logger.Debug("cache miss", "cache_key", cacheKey)

		// Create a custom response writer to capture the response
		writer := &responseCapture{
			ResponseWriter: c.Writer,
			body:           make([]byte, 0),
		}
		c.Writer = writer

		// Continue to handler
		c.Next()

		// Cache the response if it was successful and contains audio data
		if c.Writer.Status() == http.StatusOK && len(writer.body) > 1024 {
			if err := s.ginCache.Set(cacheKey, writer.body, duration); err != nil {
				s.logger.Warn("failed to cache response", "cache_key", cacheKey, "error", err)
			} else {
				s.logger.Debug("cached response",
					"cache_key", cacheKey,
					"size", len(writer.body),
					"duration", duration,
				)
			}
		}
	}
}

// generateNormalizedCacheKey creates a consistent cache key by normalizing URL parameters
func (s *GinServer) generateNormalizedCacheKey(c *gin.Context) string {
	// Parse and normalize query parameters
	params := c.Request.URL.Query()
	normalizedParams := url.Values{}

	for key, values := range params {
		for _, value := range values {
			// URL decode the value to normalize it
			if decoded, err := url.QueryUnescape(value); err == nil {
				normalizedParams.Add(key, decoded)
			} else {
				normalizedParams.Add(key, value)
			}
		}
	}

	// Include station parameter from path
	station := c.Param("station")

	// Generate consistent cache key
	cacheKey := fmt.Sprintf("clips:%s:%s", station, normalizedParams.Encode())
	return cacheKey
}

// responseCapture captures response data for caching
type responseCapture struct {
	gin.ResponseWriter
	body []byte
}

func (w *responseCapture) Write(data []byte) (int, error) {
	w.body = append(w.body, data...)
	return w.ResponseWriter.Write(data)
}

// formatFileSize converts bytes to human-readable format
func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// getRecordingDuration uses audio.ProbeFile to get the actual duration of a recording file
func (s *GinServer) getRecordingDuration(recordingPath string) (string, error) {
	audioInfo, err := audio.ProbeFile(recordingPath)
	if err != nil {
		return "", err
	}
	return audioInfo.DurationString, nil
}

// Custom logging middleware
func (s *GinServer) customLoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		method := c.Request.Method
		statusCode := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		s.logger.HTTPRequest(method, path, statusCode, latency, "gin-request")
	}
}

func (s *GinServer) validateStation(c *gin.Context, stationName string) bool {
	if _, exists := s.config.Stations[stationName]; !exists {
		s.apiError(c, http.StatusNotFound, "Stream not found", fmt.Sprintf("Stream '%s' does not exist", stationName))
		return false
	}
	return true
}

func (s *GinServer) validateRecordingExists(c *gin.Context, stationName, timestamp string) (string, audio.Format, bool) {
	recordingPath, format, exists := utils.FindRecordingFile(s.config.RecordingsDirectory, stationName, timestamp)
	if !exists {
		s.apiError(c, http.StatusNotFound, "Recording not found", fmt.Sprintf("Recording '%s' does not exist", timestamp))
		return "", audio.Format{}, false
	}
	return recordingPath, format, true
}

func (s *GinServer) apiResponse(c *gin.Context, status int, data interface{}, count int) {
	c.JSON(status, gin.H{
		"success": status < 400,
		"data":    data,
		"meta": gin.H{
			"timestamp": time.Now(),
			"version":   version.Version,
			"count":     count,
		},
	})
}

func (s *GinServer) apiError(c *gin.Context, status int, message, details string) {
	c.JSON(status, gin.H{
		"success": false,
		"error": gin.H{
			"code":    status,
			"message": message,
			"details": details,
		},
		"meta": gin.H{
			"timestamp": time.Now(),
			"version":   version.Version,
		},
	})
}

func (s *GinServer) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now(),
		"version":   version.Version,
		"uptime":    time.Since(startTime).String(),
	})
}

func (s *GinServer) readinessHandler(c *gin.Context) {
	checks := []gin.H{
		{"name": "cache", "status": "ok"},
		{"name": "storage", "status": "ok"},
	}

	if _, err := os.Stat(s.config.RecordingsDirectory); os.IsNotExist(err) {
		checks[1]["status"] = "error"
		checks[1]["message"] = "Recording directory not accessible"
	}

	allReady := true
	for _, check := range checks {
		if check["status"] != "ok" {
			allReady = false
			break
		}
	}

	status := http.StatusOK
	if !allReady {
		status = http.StatusServiceUnavailable
	}

	s.apiResponse(c, status, gin.H{
		"ready":  allReady,
		"checks": checks,
	}, len(checks))
}

func (s *GinServer) stationsHandler(c *gin.Context) {
	stations := make([]gin.H, 0, len(s.config.Stations))

	for stationName, station := range s.config.Stations {
		totalSize, lastSeen, recordingCount, _ := s.calculateStationStats(stationName)

		stations = append(stations, gin.H{
			"name":          stationName,
			"url":           station.URL,
			"status":        "active",
			"last_recorded": utils.ToAPIStringOrEmpty(lastSeen, s.config.Timezone),
			"recordings":    recordingCount,
			"total_size":    totalSize,
			"keep_days":     s.config.GetStationKeepDays(stationName),
			"has_metadata":  station.MetadataURL != "",
		})
	}

	s.apiResponse(c, http.StatusOK, gin.H{"stations": stations}, len(stations))
}

func (s *GinServer) stationDetailsHandler(c *gin.Context) {
	stationName := c.Param("station")

	if !s.validateStation(c, stationName) {
		return
	}

	station := s.config.Stations[stationName]
	totalSize, lastSeen, recordingCount, _ := s.calculateStationStats(stationName)

	s.apiResponse(c, http.StatusOK, gin.H{
		"name":          stationName,
		"url":           station.URL,
		"status":        "active",
		"last_recorded": utils.ToAPIStringOrEmpty(lastSeen, s.config.Timezone),
		"recordings":    recordingCount,
		"total_size":    totalSize,
		"keep_days":     s.config.GetStationKeepDays(stationName),
		"has_metadata":  station.MetadataURL != "",
	}, 1)
}

func (s *GinServer) recordingsHandler(c *gin.Context) {
	stationName := c.Param("station")

	if !s.validateStation(c, stationName) {
		return
	}

	recordings, err := s.getRecordings(stationName)
	if err != nil {
		s.logger.Error("failed to list recordings", "error", err)
		s.apiError(c, http.StatusInternalServerError, "Failed to list recordings", err.Error())
		return
	}

	recordingsResponse := make([]gin.H, len(recordings))
	for i, recording := range recordings {
		startTime, _ := utils.ParseTimestamp(recording.Timestamp, s.config.Timezone)
		endTime := startTime.Add(time.Hour)
		hasMetadata := s.hasMetadataForRecording(stationName, recording.Timestamp)

		// Get actual duration from ffprobe
		recordingPath, _, exists := utils.FindRecordingFile(s.config.RecordingsDirectory, stationName, recording.Timestamp)
		duration := "unknown"
		if exists {
			if dur, err := s.getRecordingDuration(recordingPath); err == nil {
				duration = dur
			} else {
				s.logger.Warn("failed to get recording duration", "path", recordingPath, "error", err)
			}
		}

		recordingsResponse[i] = gin.H{
			"timestamp":    recording.Timestamp,
			"start_time":   utils.ToAPIString(startTime, s.config.Timezone),
			"end_time":     utils.ToAPIString(endTime, s.config.Timezone),
			"duration":     duration,
			"size":         recording.Size,
			"size_human":   formatFileSize(recording.Size),
			"has_metadata": hasMetadata,
			"urls":         s.buildRecordingURLs(stationName, recording.Timestamp, hasMetadata),
		}
	}

	s.apiResponse(c, http.StatusOK, gin.H{
		"recordings": recordingsResponse,
		"station":    stationName,
	}, len(recordingsResponse))
}

func (s *GinServer) recordingHandler(c *gin.Context) {
	stationName := c.Param("station")
	timestamp := c.Param("timestamp")

	if !s.validateStation(c, stationName) {
		return
	}

	recordingPath, _, exists := s.validateRecordingExists(c, stationName, timestamp)
	if !exists {
		return
	}

	stat, err := os.Stat(recordingPath)
	if err != nil {
		s.apiError(c, http.StatusInternalServerError, "Failed to get recording info", err.Error())
		return
	}

	startTime, err := utils.ParseTimestamp(timestamp, s.config.Timezone)
	if err != nil {
		s.apiError(c, http.StatusBadRequest, "Invalid timestamp format", err.Error())
		return
	}
	endTime := startTime.Add(time.Hour)

	hasMetadata := s.hasMetadataForRecording(stationName, timestamp)

	// Get actual duration from ffprobe
	duration, err := s.getRecordingDuration(recordingPath)
	if err != nil {
		s.logger.Warn("failed to get recording duration", "path", recordingPath, "error", err)
		duration = "unknown"
	}

	recording := gin.H{
		"timestamp":    timestamp,
		"start_time":   utils.ToAPIString(startTime, s.config.Timezone),
		"end_time":     utils.ToAPIString(endTime, s.config.Timezone),
		"duration":     duration,
		"size":         stat.Size(),
		"size_human":   formatFileSize(stat.Size()),
		"has_metadata": hasMetadata,
		"urls":         s.buildRecordingURLs(stationName, timestamp, hasMetadata),
	}

	if hasMetadata {
		if metadata, err := s.metadata.GetMetadata(utils.StationDirectory(s.config.RecordingsDirectory, stationName), timestamp); err == nil {
			recording["metadata"] = metadata
		}
	}

	s.apiResponse(c, http.StatusOK, recording, 1)
}

func (s *GinServer) playRecordingHandler(c *gin.Context) {
	stationName := c.Param("station")
	timestamp := c.Param("timestamp")

	if !s.validateStation(c, stationName) {
		return
	}

	recordingPath, format, exists := s.validateRecordingExists(c, stationName, timestamp)
	if !exists {
		return
	}

	c.Header("Content-Type", format.MimeType)
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s_%s%s\"", stationName, timestamp, format.Extension))
	c.File(recordingPath)
}

func (s *GinServer) downloadRecordingHandler(c *gin.Context) {
	stationName := c.Param("station")
	timestamp := c.Param("timestamp")

	if !s.validateStation(c, stationName) {
		return
	}

	recordingPath, format, exists := s.validateRecordingExists(c, stationName, timestamp)
	if !exists {
		return
	}

	c.Header("Content-Type", format.MimeType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_%s%s\"", stationName, timestamp, format.Extension))
	c.File(recordingPath)
}

func (s *GinServer) metadataHandler(c *gin.Context) {
	stationName := c.Param("station")
	timestamp := c.Param("timestamp")

	if !s.validateStation(c, stationName) {
		return
	}

	metadata, err := s.metadata.GetMetadata(utils.StationDirectory(s.config.RecordingsDirectory, stationName), timestamp)
	if err != nil {
		s.apiError(c, http.StatusNotFound, "Metadata not found", err.Error())
		return
	}

	s.apiResponse(c, http.StatusOK, gin.H{
		"station":    stationName,
		"timestamp":  timestamp,
		"metadata":   metadata,
		"fetched_at": utils.ToAPIString(utils.NowInTimezone(s.config.Timezone), s.config.Timezone),
	}, 1)
}

func (s *GinServer) audioClipHandler(c *gin.Context) {
	stationName := c.Param("station")

	if !s.validateStation(c, stationName) {
		return
	}

	var req AudioClipRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		s.apiError(c, http.StatusBadRequest, "Invalid parameters", err.Error())
		return
	}

	// Always log parameter details for debugging URL encoding issues
	s.logger.Info("audio clip request",
		"station", stationName,
		"raw_start", req.Start,
		"raw_end", req.End,
		"raw_query", c.Request.URL.RawQuery,
		"full_url", c.Request.URL.String(),
	)

	if s.config.Debug {
		s.logger.Debug("generating audio clip", "station", stationName, "start", req.Start, "end", req.End)
	}

	startTime, err := utils.ParseTimestampAsTimezone(req.Start, s.config.Timezone)
	if err != nil {
		s.apiError(c, http.StatusBadRequest, "Invalid start time format", err.Error())
		return
	}

	endTime, err := utils.ParseTimestampAsTimezone(req.End, s.config.Timezone)
	if err != nil {
		s.apiError(c, http.StatusBadRequest, "Invalid end time format", err.Error())
		return
	}

	if endTime.Before(startTime) || endTime.Equal(startTime) {
		s.apiError(c, http.StatusBadRequest, "End time must be after start time", "")
		return
	}

	s.logger.Info("attempting to generate audio clip",
		"station", stationName,
		"start_time", startTime,
		"end_time", endTime,
		"duration", endTime.Sub(startTime),
	)

	clip, clipFormat, err := s.generateAudioClipFromHourlyRecording(stationName, startTime, endTime)
	if err != nil {
		s.logger.Error("failed to generate audio clip", "error", err)
		s.apiError(c, http.StatusInternalServerError, "Failed to generate audio clip", err.Error())
		return
	}

	// Get actual duration of the generated clip
	audioInfo, err := audio.ProbeFile(clip)
	if err != nil {
		s.logger.Error("failed to get clip duration", "clip", clip, "error", err)
		s.apiError(c, http.StatusInternalServerError, "Failed to get clip duration", err.Error())
		return
	}

	durationSeconds := audioInfo.Duration.Seconds()

	// Log details about the generated clip
	if stat, statErr := os.Stat(clip); statErr == nil {
		s.logger.Info("generated audio clip",
			"station", stationName,
			"clip_path", clip,
			"clip_size", stat.Size(),
			"clip_format", clipFormat.Extension,
			"duration", audioInfo.Duration,
			"duration_seconds", durationSeconds,
		)
	} else {
		s.logger.Error("failed to stat generated clip", "error", statErr, "clip_path", clip)
	}

	// Ensure cleanup of temp file after serving
	defer func() {
		if utils.FileExists(clip) {
			if err := os.Remove(clip); err != nil {
				s.logger.Warn("failed to cleanup temp clip file", "file", clip, "error", err)
			}
		}
	}()

	if s.config.Debug {
		s.logger.Debug("serving audio clip", "station", stationName, "file", clip, "format", clipFormat.Extension, "duration", audioInfo.Duration)
	}

	// Set headers - only use X-Audio-Duration as a custom header for API convenience
	c.Header("Content-Type", clipFormat.MimeType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_%s_%s%s\"",
		stationName, startTime.Format("2006-01-02-15-04-05"), endTime.Format("2006-01-02-15-04-05"), clipFormat.Extension))
	c.Header("X-Audio-Duration", fmt.Sprintf("%.3f", durationSeconds))
	c.File(clip)
}

func (s *GinServer) systemStatsHandler(c *gin.Context) {
	stationStats := make(map[string]gin.H)
	totalRecordings := 0
	totalSize := int64(0)

	for stationName := range s.config.Stations {
		streamSize, lastActive, recordingCount, _ := s.calculateStationStats(stationName)

		stationStats[stationName] = gin.H{
			"recordings":    recordingCount,
			"size_bytes":    streamSize,
			"last_recorded": lastActive,
		}

		totalRecordings += recordingCount
		totalSize += streamSize
	}

	s.apiResponse(c, http.StatusOK, gin.H{
		"uptime":           time.Since(startTime).String(),
		"total_recordings": totalRecordings,
		"total_size":       totalSize,
		"station_stats":    stationStats,
	}, 1)
}

// Helper functions (reuse from original implementation)
func (s *GinServer) calculateStationStats(stationName string) (totalSize int64, lastSeen time.Time, recordingCount int, err error) {
	recordings, err := s.getRecordings(stationName)
	if err != nil {
		return 0, time.Time{}, 0, err
	}
	if len(recordings) == 0 {
		return 0, time.Time{}, 0, nil
	}
	for _, recording := range recordings {
		totalSize += recording.Size
		if t, err := utils.ParseTimestamp(recording.Timestamp, s.config.Timezone); err == nil {
			if t.After(lastSeen) {
				lastSeen = t
			}
		}
	}
	return totalSize, lastSeen, len(recordings), nil
}

func (s *GinServer) buildRecordingURLs(stationName, timestamp string, hasMetadata bool) gin.H {
	baseURL := fmt.Sprintf("/api/v1/stations/%s/recordings/%s", stationName, timestamp)
	urls := gin.H{
		"download": baseURL + "/download",
		"playback": baseURL + "/play",
		"details":  baseURL,
	}
	if hasMetadata {
		urls["metadata"] = baseURL + "/metadata"
	}
	return urls
}

func (s *GinServer) hasMetadataForRecording(stationName, timestamp string) bool {
	metadataPath := utils.MetadataPath(s.config.RecordingsDirectory, stationName, timestamp)
	return utils.FileExists(metadataPath)
}

func (s *GinServer) getRecordings(stationName string) ([]RecordingInfo, error) {
	stationDir := utils.StationDirectory(s.config.RecordingsDirectory, stationName)
	var recordings []RecordingInfo

	err := filepath.Walk(stationDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && utils.IsAudioFile(path) {
			timestamp, format, err := utils.GetTimestampFromAudioFile(filepath.Base(path))
			if err != nil {
				// Skip files with unrecognized formats
				return nil
			}
			recordings = append(recordings, RecordingInfo{
				Timestamp: timestamp,
				Size:      info.Size(),
				Format:    format,
			})
		}
		return nil
	})

	return recordings, err
}

func (s *GinServer) generateAudioClipFromHourlyRecording(stationName string, startTime, endTime time.Time) (string, audio.Format, error) {
	loc, err := s.config.GetTimezone()
	if err != nil {
		s.logger.Error("failed to load timezone", "error", err)
		loc = time.Local
	}

	startTimeLocal := time.Date(startTime.Year(), startTime.Month(), startTime.Day(),
		startTime.Hour(), startTime.Minute(), startTime.Second(), startTime.Nanosecond(), loc)
	endTimeLocal := time.Date(endTime.Year(), endTime.Month(), endTime.Day(),
		endTime.Hour(), endTime.Minute(), endTime.Second(), endTime.Nanosecond(), loc)

	recordingHour := time.Date(startTimeLocal.Year(), startTimeLocal.Month(), startTimeLocal.Day(), startTimeLocal.Hour(), 0, 0, 0, loc)
	timestamp := utils.FormatTimestamp(recordingHour, s.config.Timezone)
	recordingPath, format, exists := utils.FindRecordingFile(s.config.RecordingsDirectory, stationName, timestamp)

	if s.config.Debug {
		s.logger.Debug("looking for hourly recording", "station", stationName, "timestamp", timestamp, "path", recordingPath, "exists", exists)
	}

	if !exists {
		return "", audio.Format{}, fmt.Errorf("hourly recording not found for %s", timestamp)
	}

	offsetFromHour := startTimeLocal.Sub(recordingHour)
	duration := endTimeLocal.Sub(startTimeLocal)

	// Generate temporary file for the clip with process ID for uniqueness
	tempFile := filepath.Join(s.config.Server.CacheDirectory, fmt.Sprintf("clip_%s_%d_%d%s", stationName, os.Getpid(), time.Now().UnixNano(), format.Extension))

	if s.config.Debug {
		s.logger.Debug("generating audio clip", "station", stationName, "offset", offsetFromHour, "duration", duration, "output", tempFile)
	}

	if err := s.extractClip(recordingPath, tempFile, offsetFromHour, duration); err != nil {
		// Clean up failed extraction file
		if utils.FileExists(tempFile) {
			if removeErr := os.Remove(tempFile); removeErr != nil {
				s.logger.Warn("failed to cleanup failed extraction file", "file", tempFile, "error", removeErr)
			}
		}
		return "", audio.Format{}, fmt.Errorf("failed to extract clip: %w", err)
	}

	return tempFile, format, nil
}

func (s *GinServer) extractClip(inputFile, outputFile string, startOffset, duration time.Duration) error {
	// Use stream copy with output-side seeking for better precision
	// Add metadata preservation to ensure proper duration info
	outputArgs := ffmpeglib.KwArgs{
		"ss":                utils.FormatDuration(startOffset),
		"t":                 utils.FormatDuration(duration),
		"c":                 "copy",
		"avoid_negative_ts": "make_zero",
		"copyts":            "",
		"map_metadata":      "0", // Copy metadata from input
		"y":                 "",
	}

	if s.config.Debug {
		s.logger.Debug("running ffmpeg extraction", "input", inputFile, "output", outputFile, "ss", utils.FormatDuration(startOffset), "t", utils.FormatDuration(duration))
	}

	err := ffmpeglib.Input(inputFile).Output(outputFile, outputArgs).OverWriteOutput().Silent(true).Run()

	if err != nil {
		s.logger.Error("ffmpeg extraction failed",
			"input", inputFile,
			"output", outputFile,
			"start_offset", utils.FormatDuration(startOffset),
			"duration", utils.FormatDuration(duration),
			"error", err,
		)
		return fmt.Errorf("failed to extract clip: %w", err)
	}

	// Verify the output file is valid and not just metadata
	if stat, err := os.Stat(outputFile); err != nil {
		return fmt.Errorf("output file not created: %w", err)
	} else if stat.Size() < 1024 {
		// If file is smaller than 1KB, it's likely just metadata
		return fmt.Errorf("output file too small (%d bytes), likely extraction failed", stat.Size())
	}

	return nil
}

// cleanupTempFiles removes old clip temp files from the cache directory
func (s *GinServer) cleanupTempFiles() {
	pattern := filepath.Join(s.config.Server.CacheDirectory, "clip_*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		s.logger.Warn("failed to glob temp files for cleanup", "error", err)
		return
	}

	cleaned := 0
	for _, file := range matches {
		if err := os.Remove(file); err != nil {
			s.logger.Warn("failed to remove temp file", "file", file, "error", err)
		} else {
			cleaned++
		}
	}

	if cleaned > 0 {
		s.logger.Info("cleaned up temp files", "count", cleaned)
	}
}
