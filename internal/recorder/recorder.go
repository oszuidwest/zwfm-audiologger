package recorder

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/logger"
	"github.com/oszuidwest/zwfm-audiologger/internal/metadata"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
	"github.com/robfig/cron/v3"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

// RecordingStats tracks recording health and performance
type RecordingStats struct {
	StartTime       time.Time
	Attempts        int
	LastError       error
	BytesRecorded   int64
	ExpectedBytes   int64
	Reconnections   int
	DetectedBitrate int // kbps detected from stream
}

// Recorder handles audio stream recording with resilience features
// Optimized struct field ordering for Go 1.24+ memory alignment
type Recorder struct {
	cron     *cron.Cron
	config   *config.Config
	logger   *logger.Logger
	metadata *metadata.Fetcher
	stats    map[string]*RecordingStats
	mu       sync.RWMutex
}

// New creates a new Recorder instance
func New(cfg *config.Config, log *logger.Logger) *Recorder {
	return &Recorder{
		config:   cfg,
		logger:   log,
		cron:     cron.New(),
		metadata: metadata.New(log),
		stats:    make(map[string]*RecordingStats),
	}
}

// StartCron starts the cron scheduler for hourly recordings
func (r *Recorder) StartCron(ctx context.Context) error {
	// Schedule recording to run every hour at minute 0
	_, err := r.cron.AddFunc("0 * * * *", func() {
		if err := r.RecordAll(ctx); err != nil {
			r.logger.Error("scheduled recording failed", "error", err)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to add cron job: %w", err)
	}

	r.cron.Start()
	r.logger.Info("Cron scheduler started - recordings will run every hour")

	// Wait for context cancellation
	<-ctx.Done()
	r.cron.Stop()
	r.logger.Info("Cron scheduler stopped")
	return nil
}

// RecordAll records all configured streams
func (r *Recorder) RecordAll(ctx context.Context) error {
	timestamp := utils.GetCurrentHour()

	// Ensure recording directory exists
	if err := utils.EnsureDir(r.config.RecordingDir); err != nil {
		return fmt.Errorf("failed to create recording directory: %w", err)
	}

	var wg sync.WaitGroup

	for streamName, stream := range r.config.Streams {
		wg.Add(1)
		go func(name string, s config.Stream) {
			defer wg.Done()

			if err := r.recordStream(ctx, name, s, timestamp); err != nil {
				r.logger.Error("recording failed", "station", name, "error", err)
			}
		}(streamName, stream)
	}

	wg.Wait()
	return nil
}

// recordStream records a single stream with resilience and retry logic
func (r *Recorder) recordStream(ctx context.Context, streamName string, stream config.Stream, timestamp string) error {
	// Initialize recording stats
	r.initStats(streamName)

	// Pre-flight checks and bitrate detection
	bitrate, err := r.preflightChecks(ctx, streamName, stream)
	if err != nil {
		r.updateStats(streamName, func(s *RecordingStats) {
			s.LastError = err
		})
		return fmt.Errorf("preflight checks failed: %w", err)
	}

	// Store detected bitrate
	r.updateStats(streamName, func(s *RecordingStats) {
		s.DetectedBitrate = bitrate
	})

	// Create stream directory
	streamDir := utils.StreamDir(r.config.RecordingDir, streamName)
	if err := utils.EnsureDir(streamDir); err != nil {
		return fmt.Errorf("failed to create stream directory: %w", err)
	}

	// Start cleanup in background
	go r.cleanupOldFiles(streamName, streamDir)

	// Define output file path
	outputFile := utils.RecordingPath(r.config.RecordingDir, streamName, timestamp)

	// Check if output file already exists (duplicate prevention)
	if utils.FileExists(outputFile) {
		// Check if existing file is complete and valid
		if r.validateRecording(outputFile, time.Duration(stream.RecordDuration), bitrate) {
			r.logger.Info("recording already exists", "station", streamName, "timestamp", timestamp)
			return nil
		}
		r.logger.Warn("incomplete recording exists, will retry", "station", streamName, "timestamp", timestamp)
		// Remove incomplete file
		if err := os.Remove(outputFile); err != nil {
			r.logger.Error("failed to remove incomplete file", "station", streamName, "file", outputFile, "error", err)
		}
	}

	// Start metadata fetch in background
	go r.metadata.Fetch(streamName, stream, streamDir, timestamp)

	// Start recording with retry logic
	r.logger.Info("recording started", "station", streamName, "timestamp", timestamp, "bitrate_kbps", bitrate, "duration", time.Duration(stream.RecordDuration).String())

	if err := r.recordWithRetry(ctx, stream.URL, outputFile, time.Duration(stream.RecordDuration), streamName); err != nil {
		r.updateStats(streamName, func(s *RecordingStats) {
			s.LastError = err
		})
		attempts := r.getStatsAttempts(streamName)
		r.logger.Error("recording failed", "station", streamName, "timestamp", timestamp, "attempts", attempts, "error", err)
		return fmt.Errorf("recording failed after retries: %w", err)
	}

	// Validate the completed recording
	if !r.validateRecording(outputFile, time.Duration(stream.RecordDuration), bitrate) {
		err := fmt.Errorf("recording validation failed")
		r.updateStats(streamName, func(s *RecordingStats) {
			s.LastError = err
		})
		return err
	}

	// Get file size for logging
	fileInfo, _ := os.Stat(outputFile)
	var fileSize int64
	if fileInfo != nil {
		fileSize = fileInfo.Size()
	}

	r.logger.Info("recording completed", "station", streamName, "timestamp", timestamp, "file_size", fileSize, "duration", time.Duration(stream.RecordDuration).String())
	return nil
}

// preflightChecks performs health checks and bitrate detection before recording
func (r *Recorder) preflightChecks(ctx context.Context, streamName string, stream config.Stream) (int, error) {
	// Check disk space (require at least 500MB free)
	if !r.checkDiskSpace(r.config.RecordingDir, 500*1024*1024) {
		return 0, fmt.Errorf("insufficient disk space")
	}

	// Detect stream bitrate from icecast headers
	bitrate, err := r.detectStreamBitrate(stream.URL)
	if err != nil {
		r.logger.Warn("bitrate detection failed, using default", "station", streamName, "default_kbps", 128, "error", err)
		bitrate = 128 // Default fallback
	} else {
		r.logger.Info("bitrate detected", "station", streamName, "method", "icecast-headers", "bitrate_kbps", bitrate)
	}

	return bitrate, nil
}

// checkDiskSpace checks if there's enough free disk space
func (r *Recorder) checkDiskSpace(path string, requiredBytes int64) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		r.logger.Error("failed to check disk space", "error", err)
		return false
	}

	freeBytes := int64(stat.Bavail) * int64(stat.Bsize)
	return freeBytes >= requiredBytes
}

// recordWithRetry attempts recording with exponential backoff retry
func (r *Recorder) recordWithRetry(ctx context.Context, streamURL, outputFile string, duration time.Duration, streamName string) error {
	maxRetries := 3
	baseDelay := 5 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		r.updateStats(streamName, func(s *RecordingStats) {
			s.Attempts = attempt
		})

		r.logger.Info("recording attempt", "station", streamName, "attempt", attempt, "max_attempts", maxRetries)

		// Calculate adaptive timeout based on attempt
		timeoutMultiplier := time.Duration(attempt)
		recordingCtx, cancel := context.WithTimeout(ctx, duration+(timeoutMultiplier*30*time.Second))

		err := r.startFFmpegWithContext(recordingCtx, streamURL, outputFile, duration)
		cancel()

		if err == nil {
			r.logger.Info("recording attempt successful", "station", streamName, "attempt", attempt)
			return nil
		}

		r.logger.Warn("recording attempt failed", "station", streamName, "attempt", attempt, "error", err)
		r.updateStats(streamName, func(s *RecordingStats) {
			s.LastError = err
		})

		// If not the last attempt, wait before retrying
		if attempt < maxRetries {
			retryDelay := baseDelay * time.Duration(attempt)
			r.logger.Info("waiting before retry", "station", streamName, "delay", retryDelay)
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return fmt.Errorf("all recording attempts failed")
}

// startFFmpegWithContext starts FFmpeg with proper context handling
func (r *Recorder) startFFmpegWithContext(ctx context.Context, streamURL, outputFile string, duration time.Duration) error {
	// Enhanced FFmpeg settings for resilience
	stream := ffmpeg.Input(streamURL, ffmpeg.KwArgs{
		"reconnect":               "1",
		"reconnect_at_eof":        "1",
		"reconnect_streamed":      "1",
		"reconnect_delay_max":     "60",                  // Reduced from 300 to fail faster
		"reconnect_on_http_error": "404,500,502,503,504", // Added more error codes
		"rw_timeout":              "30000000",            // Increased to 30 seconds
		"timeout":                 "60000000",            // 60 second general timeout
		"t":                       fmt.Sprintf("%.0f", duration.Seconds()),
		"threads":                 "0",  // Use optimal thread count
		"bufsize":                 "2M", // Increased buffer size
	})

	// Output with enhanced settings
	cmd := stream.Output(outputFile, ffmpeg.KwArgs{
		"c":                 "copy",
		"f":                 "mp3",
		"y":                 "",
		"avoid_negative_ts": "make_zero",
		"copyts":            "",
		"start_at_zero":     "",
	}).OverWriteOutput()

	// Create a channel to handle FFmpeg completion
	errorChan := make(chan error, 1)

	go func() {
		errorChan <- cmd.Run()
	}()

	// Wait for either completion or context cancellation
	select {
	case err := <-errorChan:
		return err
	case <-ctx.Done():
		// Context cancelled - FFmpeg process should be killed by the library
		return ctx.Err()
	}
}

// detectStreamBitrate detects bitrate from icecast stream using GET request only
func (r *Recorder) detectStreamBitrate(streamURL string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		return 0, fmt.Errorf("invalid stream URL: %w", err)
	}

	// Add icecast-compatible headers
	req.Header.Set("Icy-MetaData", "1")
	req.Header.Set("User-Agent", "AudioLogger/1.0")
	req.Header.Set("Accept", "audio/mpeg, audio/*")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("stream connection failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			r.logger.Warn("failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("stream returned status %d", resp.StatusCode)
	}

	// Check icecast-specific headers for bitrate
	if bitrate := resp.Header.Get("icy-br"); bitrate != "" {
		if br, err := strconv.Atoi(bitrate); err == nil {
			return br, nil
		}
	}

	// Check alternative header formats
	if bitrate := resp.Header.Get("ice-audio-info"); bitrate != "" {
		// Parse "ice-audio-info: bitrate=128;samplerate=44100" format
		re := regexp.MustCompile(`bitrate=(\d+)`)
		if matches := re.FindStringSubmatch(bitrate); len(matches) > 1 {
			if br, err := strconv.Atoi(matches[1]); err == nil {
				return br, nil
			}
		}
	}

	// Read a small sample to analyze MP3 frames (quick analysis)
	buffer := make([]byte, 4096) // 4KB sample - enough for several MP3 frames
	n, err := resp.Body.Read(buffer)
	if err != nil && n == 0 {
		return 0, fmt.Errorf("failed to read stream sample")
	}

	// Analyze MP3 frames for bitrate
	if bitrate, err := r.detectMP3Bitrate(buffer[:n]); err == nil {
		return bitrate, nil
	}

	return 0, fmt.Errorf("unable to detect bitrate from headers or content")
}

// detectMP3Bitrate analyzes MP3 data to detect bitrate
func (r *Recorder) detectMP3Bitrate(data []byte) (int, error) {
	// MP3 bitrate lookup table (MPEG-1 Layer III)
	bitrateTable := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}

	for i := 0; i < len(data)-4; i++ {
		// Look for MP3 frame sync (11 bits set to 1)
		if data[i] == 0xFF && (data[i+1]&0xE0) == 0xE0 {
			// Extract bitrate index from header
			bitrateIndex := (data[i+2] & 0xF0) >> 4
			if bitrateIndex > 0 && bitrateIndex < 15 {
				return bitrateTable[bitrateIndex], nil
			}
		}
	}

	return 0, fmt.Errorf("no valid MP3 frames found")
}

// validateRecording checks if a recording file is complete and valid based on detected bitrate
func (r *Recorder) validateRecording(filePath string, expectedDuration time.Duration, bitrate int) bool {
	// Check if file exists and has reasonable size
	stat, err := os.Stat(filePath)
	if err != nil {
		return false
	}

	// Calculate expected size based on detected bitrate
	// Formula: (bitrate_kbps * duration_seconds) / 8 = bytes
	// Add 10% tolerance for container overhead and variations
	expectedSizeBytes := int64(float64(bitrate*1024) * expectedDuration.Seconds() / 8.0)
	minExpectedSize := int64(float64(expectedSizeBytes) * 0.8) // 20% tolerance
	maxExpectedSize := int64(float64(expectedSizeBytes) * 1.2) // 20% tolerance

	if stat.Size() < minExpectedSize {
		r.logger.Warn("recording file smaller than expected",
			"file", filepath.Base(filePath), "actual_bytes", stat.Size(), "expected_bytes", expectedSizeBytes, "min_bytes", minExpectedSize, "bitrate_kbps", bitrate)
		return false
	}

	if stat.Size() > maxExpectedSize {
		r.logger.Warn("recording file larger than expected",
			"file", filepath.Base(filePath), "actual_bytes", stat.Size(), "expected_bytes", expectedSizeBytes, "max_bytes", maxExpectedSize, "bitrate_kbps", bitrate)
		// Don't fail for oversized files, just log warning
	}

	r.logger.Debug("recording validation passed",
		"file", filepath.Base(filePath), "actual_bytes", stat.Size(), "expected_bytes", expectedSizeBytes, "bitrate_kbps", bitrate)

	return true
}

// initStats initializes recording stats for a stream
func (r *Recorder) initStats(streamName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stats[streamName] = &RecordingStats{
		StartTime: time.Now(),
	}
}

// updateStats safely updates recording stats
func (r *Recorder) updateStats(streamName string, updateFunc func(*RecordingStats)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if stats, exists := r.stats[streamName]; exists {
		updateFunc(stats)
	}
}

// GetStats returns current recording stats (for monitoring/API)
func (r *Recorder) GetStats() map[string]RecordingStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]RecordingStats)
	for name, stats := range r.stats {
		result[name] = *stats
	}
	return result
}

// cleanupOldFiles removes old recording files based on retention policy
func (r *Recorder) cleanupOldFiles(streamName, streamDir string) {
	keepDays := r.config.GetStreamKeepDays(streamName)

	cutoff := time.Now().AddDate(0, 0, -keepDays)

	err := filepath.Walk(streamDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				r.logger.Warn("failed to remove old file", "station", streamName, "file", path, "error", err)
			} else {
				r.logger.Debug("removed old file", "station", streamName, "file", path)
			}
		}

		return nil
	})

	if err != nil {
		r.logger.Error("cleanup failed", "station", streamName, "error", err)
	} else {
		r.logger.Info("cleanup completed", "station", streamName)
	}
}

// getStatsAttempts returns the number of attempts for a stream
func (r *Recorder) getStatsAttempts(streamName string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if stats, exists := r.stats[streamName]; exists {
		return stats.Attempts
	}
	return 0
}
