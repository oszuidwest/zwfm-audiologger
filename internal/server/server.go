package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/logger"
	"github.com/oszuidwest/zwfm-audiologger/internal/metadata"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

type Server struct {
	config   *config.Config
	logger   *logger.Logger
	metadata *metadata.Fetcher
	server   *http.Server
	cache    *Cache
}

func New(cfg *config.Config, log *logger.Logger) *Server {
	cache := NewCache(cfg.Server.CacheDir, time.Duration(cfg.Server.CacheTTL))

	return &Server{
		config:   cfg,
		logger:   log,
		metadata: metadata.New(log),
		cache:    cache,
	}
}

func (s *Server) Start(ctx context.Context, port string) error {
	if err := s.cache.Init(); err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}

	go s.startCacheCleanup(ctx)

	router := mux.NewRouter()

	router.Use(s.loggingMiddleware)
	router.Use(s.corsMiddleware)

	api := router.PathPrefix("/api/v1").Subrouter()

	router.HandleFunc("/health", s.healthHandler).Methods("GET")
	router.HandleFunc("/ready", s.readinessHandler).Methods("GET")

	api.HandleFunc("/system/cache", s.cacheStatsHandler).Methods("GET")
	api.HandleFunc("/system/stats", s.systemStatsHandler).Methods("GET")

	api.HandleFunc("/streams", s.streamsHandler).Methods("GET")
	api.HandleFunc("/streams/{stream}", s.streamDetailsHandler).Methods("GET")

	api.HandleFunc("/streams/{stream}/recordings", s.recordingsHandler).Methods("GET")
	api.HandleFunc("/streams/{stream}/recordings/{timestamp}", s.recordingHandler).Methods("GET")
	api.HandleFunc("/streams/{stream}/recordings/{timestamp}/metadata", s.metadataHandler).Methods("GET")
	api.HandleFunc("/streams/{stream}/recordings/{timestamp}/download", s.downloadRecordingHandler).Methods("GET")

	api.HandleFunc("/streams/{stream}/segments", s.audioSegmentHandler).Methods("GET")

	s.server = &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  time.Duration(s.config.Server.ReadTimeout),
		WriteTimeout: time.Duration(s.config.Server.WriteTimeout),
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(s.config.Server.ShutdownTimeout))
	defer cancel()

	return s.server.Shutdown(shutdownCtx)
}

func (s *Server) audioSegmentHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamName := vars["stream"]

	if _, exists := s.config.Streams[streamName]; !exists {
		s.writeAPIError(w, http.StatusNotFound, "Stream not found", "")
		return
	}

	startTimeStr := r.URL.Query().Get("start")
	endTimeStr := r.URL.Query().Get("end")

	if startTimeStr == "" || endTimeStr == "" {
		s.writeAPIError(w, http.StatusBadRequest, "start and end parameters are required", "")
		return
	}

	startTime, err := utils.ParseTimestampAsTimezone(startTimeStr, s.config.Timezone)
	if err != nil {
		s.writeAPIError(w, http.StatusBadRequest, "Invalid start time format", err.Error())
		return
	}

	endTime, err := utils.ParseTimestampAsTimezone(endTimeStr, s.config.Timezone)
	if err != nil {
		s.writeAPIError(w, http.StatusBadRequest, "Invalid end time format", err.Error())
		return
	}

	if endTime.Before(startTime) || endTime.Equal(startTime) {
		s.writeAPIError(w, http.StatusBadRequest, "End time must be after start time", "")
		return
	}

	segment, err := s.generateAudioSegmentFromHourlyRecording(streamName, startTime, endTime)
	if err != nil {
		s.logger.Error("failed to generate audio segment", "error", err)
		s.writeAPIError(w, http.StatusInternalServerError, "Failed to generate audio segment", err.Error())
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_%s_%s.mp3\"",
		streamName, startTime.Format("2006-01-02-15-04-05"), endTime.Format("2006-01-02-15-04-05")))

	http.ServeFile(w, r, segment)
}

// generateAudioSegmentFromHourlyRecording extracts audio segments from hourly recordings
func (s *Server) generateAudioSegmentFromHourlyRecording(streamName string, startTime, endTime time.Time) (string, error) {
	if cachedPath, found := s.cache.GetCachedSegment(streamName, s.config.Timezone, startTime, endTime); found {
		s.logger.Debug("serving cached segment", "stream", streamName)
		return cachedPath, nil
	}

	// Treat all request times as recording timezone (same as recordings)
	// Simple: no timezone conversion, no standards confusion
	loc, err := s.config.GetTimezone()
	if err != nil {
		s.logger.Error("failed to load timezone", "error", err)
		loc = time.Local // Fallback to server timezone
	}

	// Parse times as Amsterdam time regardless of input timezone
	startTimeLocal := time.Date(startTime.Year(), startTime.Month(), startTime.Day(),
		startTime.Hour(), startTime.Minute(), startTime.Second(), startTime.Nanosecond(), loc)
	endTimeLocal := time.Date(endTime.Year(), endTime.Month(), endTime.Day(),
		endTime.Hour(), endTime.Minute(), endTime.Second(), endTime.Nanosecond(), loc)

	s.logger.Debug("timezone handling", "start_time", utils.ToAPIString(startTime, s.config.Timezone), "end_time", utils.ToAPIString(endTime, s.config.Timezone))

	// Find the recording hour by truncating to hour boundary (00:00 of that hour)
	// Example: 2024-01-15 14:37:23 becomes 2024-01-15 14:00:00
	recordingHour := time.Date(startTimeLocal.Year(), startTimeLocal.Month(), startTimeLocal.Day(), startTimeLocal.Hour(), 0, 0, 0, loc)
	timestamp := utils.FormatTimestamp(recordingHour, s.config.Timezone)

	s.logger.Debug("looking for recording", "timestamp", timestamp)

	recordingPath := utils.RecordingPath(s.config.RecordingDir, streamName, timestamp)

	if !utils.FileExists(recordingPath) {
		return "", fmt.Errorf("hourly recording not found for %s", timestamp)
	}

	// Calculate offset from start of hour and duration of requested segment
	// Example: if recording starts at 14:00 and request is 14:15-14:45, offset=15m, duration=30m
	offsetFromHour := startTimeLocal.Sub(recordingHour)
	duration := endTimeLocal.Sub(startTimeLocal)

	tempFile := filepath.Join(s.config.Server.CacheDir, fmt.Sprintf(".tmp_segment_%s_%d.mp3", streamName, time.Now().UnixNano()))

	if err := s.extractSegment(recordingPath, tempFile, offsetFromHour, duration); err != nil {
		return "", fmt.Errorf("failed to extract segment: %w", err)
	}

	cachedPath, err := s.cache.CacheSegment(streamName, s.config.Timezone, startTime, endTime, tempFile)
	if err != nil {
		s.logger.Warn("failed to cache segment", "error", err)
		// Schedule cleanup for temp file since caching failed
		go func() {
			time.Sleep(10 * time.Second)
			if err := os.Remove(tempFile); err != nil {
				s.logger.Debug("failed to cleanup temporary segment file", "error", err)
			}
		}()
		return tempFile, nil
	}

	s.logger.Debug("generated and cached new segment", "stream", streamName)
	return cachedPath, nil
}

func (s *Server) startCacheCleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cache.Cleanup()
			s.logger.Debug("Cache cleanup completed")
		}
	}
}

func (s *Server) cacheStatsHandler(w http.ResponseWriter, r *http.Request) {
	stats := s.cache.GetCacheStats()
	s.writeJSON(w, http.StatusOK, stats)
}

type BasicRecording struct {
	Timestamp string
	Size      int64
}

func (s *Server) getRecordings(streamName string) ([]BasicRecording, error) {
	streamDir := utils.StreamDir(s.config.RecordingDir, streamName)
	var recordings []BasicRecording

	err := filepath.Walk(streamDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".mp3") {
			timestamp := strings.TrimSuffix(filepath.Base(path), ".mp3")
			recordings = append(recordings, BasicRecording{
				Timestamp: timestamp,
				Size:      info.Size(),
			})
		}

		return nil
	})

	return recordings, err
}

// extractSegment uses FFmpeg to extract a time-range segment from a recording
// Uses stream copy (no re-encoding) for fast, lossless extraction
func (s *Server) extractSegment(inputFile, outputFile string, startOffset, duration time.Duration) error {
	err := ffmpeg.Input(inputFile, ffmpeg.KwArgs{
		"ss": utils.FormatDuration(startOffset), // Seek to start position
		"t":  utils.FormatDuration(duration),    // Extract for specified duration
	}).Output(outputFile, ffmpeg.KwArgs{
		"c":                 "copy",      // Stream copy (no re-encoding)
		"avoid_negative_ts": "make_zero", // Handle potential timestamp issues
		"y":                 "",          // Overwrite output without asking
	}).OverWriteOutput().Silent(true).Run()

	if err != nil {
		return fmt.Errorf("ffmpeg extraction failed: %w", err)
	}

	return nil
}
