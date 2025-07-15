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
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/logger"
	"github.com/oszuidwest/zwfm-audiologger/internal/metadata"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
	"github.com/oszuidwest/zwfm-audiologger/internal/version"
	"github.com/robfig/cron/v3"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

type RecordingStats struct {
	StartTime       time.Time
	Attempts        int
	LastError       error
	BytesRecorded   int64
	ExpectedBytes   int64
	Reconnections   int
	DetectedBitrate int
}

type Recorder struct {
	cron     *cron.Cron
	config   *config.Config
	logger   *logger.Logger
	metadata *metadata.Fetcher
	stats    map[string]*RecordingStats
	mu       sync.RWMutex
}

// New returns a new Recorder with the provided configuration and logger.
func New(cfg *config.Config, log *logger.Logger) *Recorder {
	return &Recorder{
		config:   cfg,
		logger:   log,
		cron:     cron.New(),
		metadata: metadata.New(log),
		stats:    make(map[string]*RecordingStats),
	}
}

// StartCron starts the cron scheduler with hourly recording jobs.
// It blocks until ctx is canceled.
func (r *Recorder) StartCron(ctx context.Context) error {
	// Cron pattern "0 * * * *" triggers at minute 0 of every hour
	// This ensures recordings start precisely at hour boundaries (00:00, 01:00, etc.)
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

	<-ctx.Done()
	r.cron.Stop()
	r.logger.Info("Cron scheduler stopped")
	return nil
}

// RecordAll records audio from all configured stations concurrently.
func (r *Recorder) RecordAll(ctx context.Context) error {
	timestamp := utils.GetCurrentHour(r.config.Timezone)
	if err := utils.EnsureDirectory(r.config.RecordingsDirectory); err != nil {
		return fmt.Errorf("failed to create recording directory: %w", err)
	}

	var wg sync.WaitGroup

	for stationName, station := range r.config.Stations {
		wg.Add(1)
		go func(name string, s config.Station) {
			defer wg.Done()

			if err := r.recordAudioStream(ctx, name, s, timestamp); err != nil {
				r.logger.Error("recording failed", "station", name, "error", err)
			}
		}(stationName, station)
	}

	wg.Wait()
	return nil
}

// recordAudioStream handles the complete recording process for a single station.
func (r *Recorder) recordAudioStream(ctx context.Context, stationName string, station config.Station, timestamp string) error {
	r.initStats(stationName)

	bitrate, err := r.validateStreamConnection(ctx, stationName, station)
	if err != nil {
		r.updateStats(stationName, func(s *RecordingStats) {
			s.LastError = err
		})
		return fmt.Errorf("preflight checks failed: %w", err)
	}

	r.updateStats(stationName, func(s *RecordingStats) {
		s.DetectedBitrate = bitrate
	})

	stationDir := utils.StationDirectory(r.config.RecordingsDirectory, stationName)
	if err := utils.EnsureDirectory(stationDir); err != nil {
		return fmt.Errorf("failed to create station directory: %w", err)
	}

	go r.cleanupOldFiles(stationName, stationDir)

	outputFile := utils.RecordingPath(r.config.RecordingsDirectory, stationName, timestamp)

	// Check if recording already exists and is valid
	if utils.FileExists(outputFile) {
		if r.validateRecording(outputFile, time.Duration(station.RecordDuration), bitrate) {
			r.logger.Info("recording already exists", "station", stationName, "timestamp", timestamp)
			return nil
		}
		r.logger.Warn("incomplete recording exists, will retry", "station", stationName, "timestamp", timestamp)
		if err := os.Remove(outputFile); err != nil {
			r.logger.Error("failed to remove incomplete file", "station", stationName, "file", outputFile, "error", err)
		}
	}

	go r.metadata.FetchMetadata(stationName, station, stationDir, timestamp)

	r.logger.Info("recording started", "station", stationName, "timestamp", timestamp, "bitrate_kbps", bitrate, "duration", time.Duration(station.RecordDuration).String())

	if err := r.recordWithRetry(ctx, station.URL, outputFile, time.Duration(station.RecordDuration), stationName); err != nil {
		r.updateStats(stationName, func(s *RecordingStats) {
			s.LastError = err
		})
		attempts := r.getStatsAttempts(stationName)
		r.logger.Error("recording failed", "station", stationName, "timestamp", timestamp, "attempts", attempts, "error", err)
		return fmt.Errorf("recording failed after retries: %w", err)
	}

	if !r.validateRecording(outputFile, time.Duration(station.RecordDuration), bitrate) {
		err := fmt.Errorf("recording validation failed")
		r.updateStats(stationName, func(s *RecordingStats) {
			s.LastError = err
		})
		return err
	}

	fileInfo, _ := os.Stat(outputFile)
	var fileSize int64
	if fileInfo != nil {
		fileSize = fileInfo.Size()
	}

	r.logger.Info("recording completed", "station", stationName, "timestamp", timestamp, "file_size", fileSize, "duration", time.Duration(station.RecordDuration).String())
	return nil
}

func (r *Recorder) validateStreamConnection(ctx context.Context, stationName string, station config.Station) (int, error) {
	bitrate, err := r.detectStreamBitrate(station.URL)
	if err != nil {
		r.logger.Warn("bitrate detection failed, station may be inaccessible", "station", stationName, "stream_url", station.URL, "error", err)
		r.logger.Info("using default bitrate for recording attempt", "station", stationName, "default_kbps", 128)
		bitrate = 128 // Fallback when detection fails
	} else {
		r.logger.Info("bitrate detected", "station", stationName, "method", "icecast-headers", "bitrate_kbps", bitrate, "stream_url", station.URL)
	}

	return bitrate, nil
}

// recordWithRetry implements exponential backoff retry pattern for robust recording
// Retries up to 3 times with delays of 5s, 10s, 15s to handle transient network issues
func (r *Recorder) recordWithRetry(ctx context.Context, streamURL, outputFile string, duration time.Duration, stationName string) error {
	maxRetries := 3
	baseDelay := 5 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		r.updateStats(stationName, func(s *RecordingStats) {
			s.Attempts = attempt
		})

		r.logger.Info("recording attempt", "station", stationName, "attempt", attempt, "max_attempts", maxRetries)

		// Adaptive timeout: base recording duration + (attempt * 30s)
		// Gives more time for subsequent attempts to handle slow connections
		timeoutMultiplier := time.Duration(attempt)
		recordingCtx, cancel := context.WithTimeout(ctx, duration+(timeoutMultiplier*30*time.Second))

		err := r.startAudioRecording(recordingCtx, streamURL, outputFile, duration)
		cancel()

		if err == nil {
			r.logger.Info("recording attempt successful", "station", stationName, "attempt", attempt)
			return nil
		}

		r.logger.Warn("recording attempt failed", "station", stationName, "attempt", attempt, "error", err, "stream_url", streamURL)
		r.updateStats(stationName, func(s *RecordingStats) {
			s.LastError = err
		})
		if attempt < maxRetries {
			// Exponential backoff: 5s, 10s, 15s delays between attempts
			retryDelay := baseDelay * time.Duration(attempt)
			r.logger.Info("waiting before retry", "station", stationName, "delay", retryDelay)
			select {
			case <-time.After(retryDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return fmt.Errorf("all recording attempts failed")
}

// startAudioRecording configures FFmpeg with resilient settings for icecast streams
func (r *Recorder) startAudioRecording(ctx context.Context, streamURL, outputFile string, duration time.Duration) error {
	stream := ffmpeg.Input(streamURL, ffmpeg.KwArgs{
		"reconnect":               "1",                                     // Enable automatic reconnection
		"reconnect_at_eof":        "1",                                     // Reconnect when stream ends unexpectedly
		"reconnect_streamed":      "1",                                     // Allow reconnection for streaming protocols
		"reconnect_delay_max":     "60",                                    // Max 60s delay between reconnect attempts
		"reconnect_on_http_error": "404,500,502,503,504",                   // HTTP errors that trigger reconnection
		"rw_timeout":              "30000000",                              // 30s read/write timeout (microseconds)
		"timeout":                 "60000000",                              // 60s overall timeout (microseconds)
		"t":                       fmt.Sprintf("%.0f", duration.Seconds()), // Recording duration
		"user_agent":              version.UserAgent(),                     // Identify as audio logger
	})

	cmd := stream.Output(outputFile, ffmpeg.KwArgs{
		"c":                 "copy",      // Stream copy (no re-encoding)
		"f":                 "mp3",       // Force MP3 format
		"avoid_negative_ts": "make_zero", // Handle negative timestamps
		"y":                 "",          // Overwrite output file without asking
	}).OverWriteOutput()

	errorChan := make(chan error, 1)

	go func() {
		cmdErr := cmd.Run()
		if cmdErr != nil {
			r.logger.Debug("ffmpeg command failed", "error", cmdErr, "stream_url", streamURL, "output_file", outputFile)
		}
		errorChan <- cmdErr
	}()

	select {
	case err := <-errorChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// detectStreamBitrate performs a multi-stage bitrate detection process:
// 1. Check icecast "icy-br" header (most reliable)
// 2. Parse "ice-audio-info" header for bitrate parameter
// 3. Analyze first 4KB of MP3 stream for frame headers (fallback)
// Returns bitrate in kbps for accurate file size validation
func (r *Recorder) detectStreamBitrate(streamURL string) (int, error) {
	// 8-second timeout balances reliability with startup speed
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		return 0, fmt.Errorf("invalid stream URL: %w", err)
	}

	// Icy-MetaData=1 requests icecast server to include metadata headers
	// These headers often contain bitrate information
	req.Header.Set("Icy-MetaData", "1")
	req.Header.Set("User-Agent", version.UserAgent())
	req.Header.Set("Accept", "audio/mpeg, audio/*")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("station connection failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			r.logger.Warn("failed to close response body", "error", err)
		}
	}()

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("stream returned status %d", resp.StatusCode)
	}

	if bitrate := resp.Header.Get("icy-br"); bitrate != "" {
		if br, err := strconv.Atoi(bitrate); err == nil {
			return br, nil
		}
	}

	if bitrate := resp.Header.Get("ice-audio-info"); bitrate != "" {
		re := regexp.MustCompile(`bitrate=(\d+)`)
		if matches := re.FindStringSubmatch(bitrate); len(matches) > 1 {
			if br, err := strconv.Atoi(matches[1]); err == nil {
				return br, nil
			}
		}
	}

	buffer := make([]byte, 4096)
	n, err := resp.Body.Read(buffer)
	if err != nil && n == 0 {
		return 0, fmt.Errorf("failed to read stream sample")
	}

	if bitrate, err := r.detectMP3Bitrate(buffer[:n]); err == nil {
		return bitrate, nil
	}

	return 0, fmt.Errorf("unable to detect bitrate from headers or content")
}

// detectMP3Bitrate analyzes raw MP3 data to extract bitrate from frame headers
// Uses MPEG-1 Layer III specification for frame parsing
func (r *Recorder) detectMP3Bitrate(data []byte) (int, error) {
	// MPEG-1 Layer III bitrate table (indexed by 4-bit bitrate field)
	// Index 0 and 15 are invalid, indices 1-14 map to standard bitrates
	bitrateTable := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}

	for i := 0; i < len(data)-4; i++ {
		// MP3 frame sync: 0xFF followed by first 3 bits = 111 (0xE0)
		// This identifies the start of a valid MP3 frame header
		if data[i] == 0xFF && (data[i+1]&0xE0) == 0xE0 {
			// Extract bitrate index from bits 4-7 of the third byte
			bitrateIndex := (data[i+2] & 0xF0) >> 4
			// Valid bitrate indices: 1-14 (0 and 15 are reserved/invalid)
			if bitrateIndex > 0 && bitrateIndex < 15 {
				return bitrateTable[bitrateIndex], nil
			}
		}
	}

	return 0, fmt.Errorf("no valid MP3 frames found")
}

// validateRecording checks if recorded file size matches expected size based on bitrate
// Uses ±20% tolerance to account for variable bitrate streams and metadata overhead
func (r *Recorder) validateRecording(filePath string, expectedDuration time.Duration, bitrate int) bool {
	stat, err := os.Stat(filePath)
	if err != nil {
		return false
	}

	// File size formula: (bitrate_kbps * 1024 bits/kbps * duration_seconds) / 8 bits/byte
	// This gives theoretical file size for constant bitrate audio
	expectedSizeBytes := int64(float64(bitrate*1024) * expectedDuration.Seconds() / 8.0)
	// Allow ±20% tolerance for VBR streams, icecast metadata, and encoding overhead
	minExpectedSize := int64(float64(expectedSizeBytes) * 0.8)
	maxExpectedSize := int64(float64(expectedSizeBytes) * 1.2)

	if stat.Size() < minExpectedSize {
		r.logger.Warn("recording file smaller than expected",
			"file", filepath.Base(filePath), "actual_bytes", stat.Size(), "expected_bytes", expectedSizeBytes, "min_bytes", minExpectedSize, "bitrate_kbps", bitrate)
		return false
	}

	if stat.Size() > maxExpectedSize {
		r.logger.Warn("recording file larger than expected",
			"file", filepath.Base(filePath), "actual_bytes", stat.Size(), "expected_bytes", expectedSizeBytes, "max_bytes", maxExpectedSize, "bitrate_kbps", bitrate)
	}

	r.logger.Debug("recording validation passed",
		"file", filepath.Base(filePath), "actual_bytes", stat.Size(), "expected_bytes", expectedSizeBytes, "bitrate_kbps", bitrate)

	return true
}

// initStats initializes recording statistics for stationName.
func (r *Recorder) initStats(stationName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stats[stationName] = &RecordingStats{
		StartTime: time.Now(),
	}
}

// updateStats safely updates recording statistics for stationName.
func (r *Recorder) updateStats(stationName string, updateFunc func(*RecordingStats)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if stats, exists := r.stats[stationName]; exists {
		updateFunc(stats)
	}
}

// GetStats returns a copy of current recording statistics for all stations.
func (r *Recorder) GetStats() map[string]RecordingStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]RecordingStats)
	for name, stats := range r.stats {
		result[name] = *stats
	}
	return result
}

// cleanupOldFiles removes files older than the configured retention period.
func (r *Recorder) cleanupOldFiles(stationName, stationDir string) {
	keepDays := r.config.GetStationKeepDays(stationName)

	cutoff := time.Now().AddDate(0, 0, -keepDays)

	err := filepath.Walk(stationDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				r.logger.Warn("failed to remove old file", "station", stationName, "file", path, "error", err)
			} else {
				r.logger.Debug("removed old file", "station", stationName, "file", path)
			}
		}

		return nil
	})

	if err != nil {
		r.logger.Error("cleanup failed", "station", stationName, "error", err)
	} else {
		r.logger.Info("cleanup completed", "station", stationName)
	}
}

// getStatsAttempts returns the number of recording attempts for stationName.
func (r *Recorder) getStatsAttempts(stationName string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if stats, exists := r.stats[stationName]; exists {
		return stats.Attempts
	}
	return 0
}
