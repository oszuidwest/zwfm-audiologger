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

// Server handles HTTP requests for audio segment serving
type Server struct {
	config   *config.Config
	logger   *logger.Logger
	metadata *metadata.Fetcher
	server   *http.Server
	cache    *Cache
}

// New creates a new Server instance
func New(cfg *config.Config, log *logger.Logger) *Server {
	cache := NewCache(cfg.Server.CacheDir, cfg.Server.CacheTTL)
	
	return &Server{
		config:   cfg,
		logger:   log,
		metadata: metadata.New(log),
		cache:    cache,
	}
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context, port string) error {
	// Initialize cache
	if err := s.cache.Init(); err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}
	
	// Start cache cleanup routine
	go s.startCacheCleanup(ctx)
	
	router := mux.NewRouter()
	
	// Add middleware
	router.Use(s.loggingMiddleware)
	router.Use(s.corsMiddleware)
	
	// Health check endpoint
	router.HandleFunc("/health", s.healthHandler).Methods("GET")
	
	// Cache stats endpoint
	router.HandleFunc("/cache/stats", s.cacheStatsHandler).Methods("GET")
	
	// List streams endpoint
	router.HandleFunc("/streams", s.streamsHandler).Methods("GET")
	
	// List recordings for a stream
	router.HandleFunc("/streams/{stream}/recordings", s.recordingsHandler).Methods("GET")
	
	// Get metadata for a recording
	router.HandleFunc("/streams/{stream}/recordings/{timestamp}/metadata", s.metadataHandler).Methods("GET")
	
	// Serve audio segments
	router.HandleFunc("/streams/{stream}/audio", s.audioSegmentHandler).Methods("GET")
	
	// Full recording download
	router.HandleFunc("/streams/{stream}/recordings/{timestamp}", s.fullRecordingHandler).Methods("GET")

	s.server = &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  s.config.Server.ReadTimeout,
		WriteTimeout: s.config.Server.WriteTimeout,
	}

	// Start server in goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Fatal("Server failed to start: ", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	
	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.config.Server.ShutdownTimeout)
	defer cancel()
	
	return s.server.Shutdown(shutdownCtx)
}

// Helper methods for file operations
func (s *Server) getStreamDir(streamName string) string {
	return utils.StreamDir(s.config.RecordingDir, streamName)
}

func (s *Server) getRecordingPath(streamName, timestamp string) string {
	return utils.RecordingPath(s.config.RecordingDir, streamName, timestamp)
}

func (s *Server) fileExists(path string) bool {
	return utils.FileExists(path)
}

// audioSegmentHandler serves audio segments based on start and end time from hourly recordings
func (s *Server) audioSegmentHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	streamName := vars["stream"]
	
	if _, exists := s.config.Streams[streamName]; !exists {
		s.writeError(w, http.StatusNotFound, "Stream not found")
		return
	}
	
	// Parse query parameters
	startTimeStr := r.URL.Query().Get("start")
	endTimeStr := r.URL.Query().Get("end")
	
	if startTimeStr == "" || endTimeStr == "" {
		s.writeError(w, http.StatusBadRequest, "start and end parameters are required (RFC3339 format)")
		return
	}
	
	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid start time format (RFC3339 required)")
		return
	}
	
	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid end time format (RFC3339 required)")
		return
	}
	
	if endTime.Before(startTime) || endTime.Equal(startTime) {
		s.writeError(w, http.StatusBadRequest, "End time must be after start time")
		return
	}
	
	// Generate audio segment from hourly recording
	segment, err := s.generateAudioSegmentFromHourlyRecording(streamName, startTime, endTime)
	if err != nil {
		s.logger.Error("Failed to generate audio segment: ", err)
		s.writeError(w, http.StatusInternalServerError, "Failed to generate audio segment: "+err.Error())
		return
	}
	
	// Serve the segment
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s_%s_%s.mp3\"", 
		streamName, startTime.Format("2006-01-02_15-04-05"), endTime.Format("2006-01-02_15-04-05")))
	
	http.ServeFile(w, r, segment)
	
	// Clean up temporary file after serving
	go func() {
		time.Sleep(10 * time.Second)
		if err := os.Remove(segment); err != nil {
			s.logger.Debug("Failed to cleanup temporary segment file: ", err)
		}
	}()
}

// generateAudioSegmentFromHourlyRecording extracts a time segment from an hourly recording
func (s *Server) generateAudioSegmentFromHourlyRecording(streamName string, startTime, endTime time.Time) (string, error) {
	// Check cache first
	if cachedPath, found := s.cache.GetCachedSegment(streamName, startTime, endTime); found {
		s.logger.Debug("Serving cached segment for ", streamName)
		return cachedPath, nil
	}
	
	// Find the hourly recording that contains the start time
	recordingHour := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), startTime.Hour(), 0, 0, 0, startTime.Location())
	timestamp := recordingHour.Format("2006-01-02_15")
	
	recordingPath := s.getRecordingPath(streamName, timestamp)
	
	if !s.fileExists(recordingPath) {
		return "", fmt.Errorf("hourly recording not found for %s", timestamp)
	}
	
	// Calculate offset from start of the hour
	offsetFromHour := startTime.Sub(recordingHour)
	duration := endTime.Sub(startTime)
	
	// Create temporary output file with Go 1.22+ improved temp file handling
	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("segment_%s_%d.mp3", streamName, time.Now().UnixNano()))
	
	// Use FFmpeg-go for reliable audio segment extraction
	if err := s.extractSegment(recordingPath, tempFile, offsetFromHour, duration); err != nil {
		return "", fmt.Errorf("failed to extract segment: %w", err)
	}
	
	// Cache the segment
	cachedPath, err := s.cache.CacheSegment(streamName, startTime, endTime, tempFile)
	if err != nil {
		s.logger.Warn("Failed to cache segment: ", err)
		return tempFile, nil // Return temp file even if caching fails
	}
	
	s.logger.Debug("Generated and cached new segment for ", streamName)
	return cachedPath, nil
}

// startCacheCleanup starts a routine to clean up expired cache entries
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

// cacheStatsHandler returns cache statistics
func (s *Server) cacheStatsHandler(w http.ResponseWriter, r *http.Request) {
	stats := s.cache.GetCacheStats()
	s.writeJSON(w, http.StatusOK, stats)
}

// getRecordings returns list of recordings for a stream
func (s *Server) getRecordings(streamName string) ([]Recording, error) {
	streamDir := s.getStreamDir(streamName)
	var recordings []Recording
	
	err := filepath.Walk(streamDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if !info.IsDir() && strings.HasSuffix(path, ".mp3") {
			timestamp := strings.TrimSuffix(filepath.Base(path), ".mp3")
			recordings = append(recordings, Recording{
				Timestamp: timestamp,
				Size:      info.Size(),
			})
		}
		
		return nil
	})
	
	return recordings, err
}

// extractSegment extracts a segment from an audio file using FFmpeg-go
func (s *Server) extractSegment(inputFile, outputFile string, startOffset, duration time.Duration) error {
	err := ffmpeg.Input(inputFile, ffmpeg.KwArgs{
		"ss": utils.FormatDuration(startOffset), // start time
		"t":  utils.FormatDuration(duration),    // duration
	}).Output(outputFile, ffmpeg.KwArgs{
		"c":                "copy",       // copy codec (no re-encoding)
		"avoid_negative_ts": "make_zero", // handle timing issues
		"y":                "",           // overwrite output file
	}).OverWriteOutput().Silent(true).Run()

	if err != nil {
		return fmt.Errorf("ffmpeg extraction failed: %w", err)
	}

	return nil
}