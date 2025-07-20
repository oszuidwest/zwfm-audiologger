// Package recorder provides audio stream recording functionality with cron scheduling,
// bitrate detection, retry logic, and format validation for radio station broadcasting.
package recorder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/audio"
	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/logger"
	"github.com/oszuidwest/zwfm-audiologger/internal/metadata"
	"github.com/oszuidwest/zwfm-audiologger/internal/peaks"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
	"github.com/oszuidwest/zwfm-audiologger/internal/version"
	"github.com/robfig/cron/v3"
	ffmpeglib "github.com/u2takey/ffmpeg-go"
)

// Simple internal recording counters
type recordingCounters struct {
	total      int
	successful int
	failed     int
}

// Recorder manages audio recording operations with cron scheduling, retry logic,
// and validation for reliable stream capture from icecast sources.
type Recorder struct {
	cron     *cron.Cron
	config   *config.Config
	logger   *logger.Logger
	metadata *metadata.Fetcher
	counters recordingCounters
}

// New returns a new Recorder with the provided configuration and logger.
func New(cfg *config.Config, log *logger.Logger) *Recorder {
	return &Recorder{
		config:   cfg,
		logger:   log,
		cron:     cron.New(),
		metadata: metadata.New(log),
	}
}

// StartCron starts the cron scheduler for hourly recordings
func (r *Recorder) StartCron(ctx context.Context) error {
	r.logger.Info("starting cron scheduler", "version", version.Version)

	_, err := r.cron.AddFunc("0 * * * *", func() {
		if err := r.RecordAll(ctx); err != nil {
			r.logger.Error("scheduled recording failed", "error", err)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to schedule recording: %w", err)
	}

	r.cron.Start()
	<-ctx.Done()
	r.cron.Stop()
	return nil
}

// RecordAll records from all configured stations concurrently
func (r *Recorder) RecordAll(ctx context.Context) error {
	if len(r.config.Stations) == 0 {
		return fmt.Errorf("no stations configured")
	}

	r.logger.Info("starting recording for all stations", "count", len(r.config.Stations))

	var wg sync.WaitGroup
	errChan := make(chan error, len(r.config.Stations))

	for stationName, stationConfig := range r.config.Stations {
		wg.Add(1)
		go func(name string, cfg config.Station) {
			defer wg.Done()
			if err := r.recordAudioStream(ctx, name, cfg); err != nil {
				errChan <- fmt.Errorf("station %s: %w", name, err)
			}
		}(stationName, stationConfig)
	}

	wg.Wait()
	close(errChan)

	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		r.logger.Error("recording completed with errors", "error_count", len(errors))
		return fmt.Errorf("recording errors: %v", errors)
	}

	r.logger.Info("all recordings completed successfully")
	return nil
}

// recordAudioStream handles the complete recording pipeline for a single station
func (r *Recorder) recordAudioStream(ctx context.Context, stationName string, station config.Station) error {
	r.logger.Info("starting recording", "station", stationName)

	// Ensure station directory exists
	stationDir := utils.StationDirectory(r.config.RecordingsDirectory, stationName)
	if err := utils.EnsureDirectory(stationDir); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", stationDir, err)
	}

	// Generate timestamp for this recording
	timestamp := utils.GetCurrentHour(r.config.Timezone)

	// Check if recording already exists in any format
	if existingPath, _, exists := utils.FindRecordingFile(r.config.RecordingsDirectory, stationName, timestamp); exists {
		r.logger.Info("recording already exists, skipping", "station", stationName, "timestamp", timestamp, "path", existingPath)
		return nil
	}

	// Detect format from actual stream using ffprobe - MANDATORY
	bitrate, streamFormat, err := r.detectStreamInfo(ctx, station.URL)
	if err != nil {
		r.updateStats(false, err)
		return fmt.Errorf("failed to detect stream format for %s: %w", stationName, err)
	}

	recordingPath := utils.RecordingPath(r.config.RecordingsDirectory, stationName, timestamp, streamFormat)

	// Clean up old recordings
	keepDays := r.config.GetStationKeepDays(stationName)
	if err := r.cleanupOldFiles(stationName, keepDays); err != nil {
		r.logger.Warn("failed to cleanup old files", "station", stationName, "error", err)
	}

	// Record with retry logic
	duration := time.Duration(station.RecordDuration)
	if err := r.recordWithRetry(ctx, stationName, station.URL, recordingPath, duration, streamFormat); err != nil {
		r.updateStats(false, err)
		return fmt.Errorf("recording failed: %w", err)
	}

	// Validate recording using ffprobe with expected bitrate
	if err := r.validateRecording(recordingPath, bitrate, streamFormat); err != nil {
		r.logger.Warn("recording validation failed, keeping file for inspection", "station", stationName, "path", recordingPath, "error", err)
		// Note: We keep the file for debugging purposes, just log the validation failure
		// Continue with metadata fetching and mark as successful recording
	}

	// Generate waveform peaks data
	r.generatePeaks(recordingPath, stationName)

	// Fetch metadata if configured
	if station.MetadataURL != "" {
		r.metadata.FetchMetadata(stationName, station, stationDir, timestamp)
	}

	r.updateStats(true, nil)
	r.logger.Info("recording completed", "station", stationName, "timestamp", timestamp, "path", recordingPath)
	return nil
}

// recordWithRetry attempts recording with exponential backoff retry logic
func (r *Recorder) recordWithRetry(ctx context.Context, stationName, streamURL, outputPath string, duration time.Duration, format audio.Format) error {
	maxRetries := 3
	baseDelay := 5 * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		r.logger.Info("recording attempt", "station", stationName, "attempt", attempt, "max_retries", maxRetries)

		err := r.startAudioRecording(ctx, streamURL, outputPath, duration, format)
		if err == nil {
			return nil // Success
		}

		lastErr = err
		r.logger.Warn("recording attempt failed", "station", stationName, "attempt", attempt, "error", err)

		if attempt < maxRetries {
			delay := time.Duration(attempt) * baseDelay
			r.logger.Info("retrying after delay", "station", stationName, "delay", delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("recording failed after %d attempts: %w", maxRetries, lastErr)
}

// startAudioRecording performs the actual FFmpeg recording
func (r *Recorder) startAudioRecording(_ context.Context, streamURL, outputPath string, duration time.Duration, format audio.Format) error {
	r.logger.Debug("starting FFmpeg recording", "url", streamURL, "output", outputPath, "duration", duration, "format", format.Name)

	inputArgs := ffmpeglib.KwArgs{
		"t":                   fmt.Sprintf("%.0f", duration.Seconds()),
		"reconnect":           "1",
		"reconnect_at_eof":    "1",
		"reconnect_streamed":  "1",
		"reconnect_delay_max": "5",
		"timeout":             "30000000",
	}

	ffmpegFormat := "mp3"
	if format.Name == "aac" {
		ffmpegFormat = "adts" // AAC ADTS format
	}

	outputArgs := ffmpeglib.KwArgs{
		"acodec": "copy",
		"f":      ffmpegFormat,
		"y":      "",
	}

	err := ffmpeglib.Input(streamURL, inputArgs).
		Output(outputPath, outputArgs).
		Run()

	if err != nil {
		return fmt.Errorf("FFmpeg recording failed: %w", err)
	}

	return nil
}

// detectStreamInfo uses audio.ProbeWithOptions to detect bitrate and format from stream URL
func (r *Recorder) detectStreamInfo(_ context.Context, streamURL string) (int, audio.Format, error) {
	r.logger.Debug("detecting stream info with ffprobe", "url", streamURL)

	// Use audio.ProbeWithOptions to analyze stream with timeout
	audioInfo, err := audio.ProbeWithOptions(streamURL, ffmpeglib.KwArgs{
		"timeout": "10000000", // 10 seconds timeout
	})
	if err != nil {
		return 0, audio.Format{}, fmt.Errorf("failed to detect stream format for %s: %w", streamURL, err)
	}

	// Bitrate is required - fail if not available
	if audioInfo.BitrateKbps <= 0 {
		return 0, audio.Format{}, fmt.Errorf("no bitrate information available for %s stream", audioInfo.Codec)
	}

	r.logger.Info("stream info detected", "method", "ffprobe", "bitrate_kbps", audioInfo.BitrateKbps, "codec", audioInfo.Codec, "format", audioInfo.Format.Name)
	return audioInfo.BitrateKbps, audioInfo.Format, nil
}

// validateRecording uses audio.ProbeFile to validate a recorded file against expected format and bitrate
func (r *Recorder) validateRecording(recordingPath string, expectedBitrate int, expectedFormat audio.Format) error {
	r.logger.Debug("validating recording with ffprobe", "path", recordingPath, "expected_bitrate", expectedBitrate, "expected_format", expectedFormat.Name)

	// Use audio.ProbeFile to analyze the recorded file
	audioInfo, err := audio.ProbeFile(recordingPath)
	if err != nil {
		return fmt.Errorf("ffprobe validation failed: %w", err)
	}

	// Check if codec matches expected format
	if audioInfo.Codec != expectedFormat.Codec {
		return fmt.Errorf("codec mismatch: expected %s, got %s", expectedFormat.Codec, audioInfo.Codec)
	}

	// Check bitrate matches expected (with tolerance)
	if audioInfo.BitrateKbps > 0 {
		tolerance := expectedBitrate / 5 // 20% tolerance
		if audioInfo.BitrateKbps < expectedBitrate-tolerance || audioInfo.BitrateKbps > expectedBitrate+tolerance {
			return fmt.Errorf("bitrate mismatch: expected ~%d kbps, got %d kbps", expectedBitrate, audioInfo.BitrateKbps)
		}
	}

	// Check duration (should be close to 1 hour for hourly recordings)
	durationSeconds := audioInfo.Duration.Seconds()
	if durationSeconds < 3500 || durationSeconds > 3700 { // 58-62 minutes tolerance
		return fmt.Errorf("recording duration %.0f seconds is outside acceptable range", durationSeconds)
	}

	r.logger.Debug("recording validation passed", "duration_seconds", durationSeconds, "codec", audioInfo.Codec, "bitrate_kbps", audioInfo.BitrateKbps)
	return nil
}

// generatePeaks generates waveform peaks data for the recorded audio file
func (r *Recorder) generatePeaks(recordingPath, stationName string) {
	r.logger.Debug("generating peaks data", "station", stationName, "path", recordingPath)

	// Create peaks generator
	peaksGen := peaks.NewGenerator(r.logger)

	// Generate and save peaks
	peaksData, err := peaksGen.Generate(recordingPath, peaks.DefaultSamplesPerPixel)
	if err != nil {
		r.logger.Error("failed to generate peaks", "station", stationName, "error", err)
		return
	}

	r.logger.Info("peaks data generated", "station", stationName, "path", peaks.GetPeaksFilePath(recordingPath), "data_points", len(peaksData.Data))
}

// cleanupOldFiles removes recordings older than the retention period
func (r *Recorder) cleanupOldFiles(stationName string, keepDays int) error {
	cutoff := time.Now().AddDate(0, 0, -keepDays)
	cleanedRecordings := 0

	// Define cleanup targets with their logic
	cleanupTargets := []struct {
		dir          string
		description  string
		isTarget     func(info os.FileInfo, path string) bool
		getTimestamp func(path string) (string, error)
		remove       func(path string) error
		counter      *int
	}{
		{
			dir:         utils.StationDirectory(r.config.RecordingsDirectory, stationName),
			description: "recording",
			isTarget: func(info os.FileInfo, path string) bool {
				return !info.IsDir() && utils.IsAudioFile(path)
			},
			getTimestamp: func(path string) (string, error) {
				timestamp, _, err := utils.GetTimestampFromAudioFile(filepath.Base(path))
				return timestamp, err
			},
			remove:  os.Remove,
			counter: &cleanedRecordings,
		},
	}

	for _, target := range cleanupTargets {
		if _, err := os.Stat(target.dir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(target.dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || !target.isTarget(info, path) {
				return err
			}

			timestamp, err := target.getTimestamp(path)
			if err != nil {
				return nil // Skip invalid items
			}

			recordingTime, err := utils.ParseTimestamp(timestamp, r.config.Timezone)
			if err != nil {
				return nil // Skip invalid timestamps
			}

			if recordingTime.Before(cutoff) {
				if err := target.remove(path); err != nil {
					r.logger.Warn("failed to remove old "+target.description, "path", path, "error", err)
				} else {
					*target.counter++
					r.logger.Debug("removed old "+target.description, "path", path, "timestamp", timestamp)

					// For audio files, also remove associated metadata and peaks files
					if target.description == "recording" {
						// Remove metadata file
						metaPath := utils.MetadataPath(r.config.RecordingsDirectory, stationName, timestamp)
						if utils.FileExists(metaPath) {
							if err := os.Remove(metaPath); err != nil {
								r.logger.Warn("failed to remove metadata file", "path", metaPath, "error", err)
							} else {
								r.logger.Debug("removed metadata file", "path", metaPath)
							}
						}

						// Remove peaks file
						peaksPath := peaks.GetPeaksFilePath(path)
						if utils.FileExists(peaksPath) {
							if err := os.Remove(peaksPath); err != nil {
								r.logger.Warn("failed to remove peaks file", "path", peaksPath, "error", err)
							} else {
								r.logger.Debug("removed peaks file", "path", peaksPath)
							}
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	if cleanedRecordings > 0 {
		r.logger.Info("cleaned up old files", "station", stationName, "removed_recordings", cleanedRecordings)
	}

	return nil
}

// updateStats updates simple recording counters
func (r *Recorder) updateStats(success bool, _ error) {
	r.counters.total++
	if success {
		r.counters.successful++
	} else {
		r.counters.failed++
	}
}
