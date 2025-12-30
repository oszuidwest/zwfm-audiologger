// Package recorder handles audio stream recording functionality.
package recorder

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/alerting"
	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
	"github.com/oszuidwest/zwfm-audiologger/internal/ffmpeg"
	"github.com/oszuidwest/zwfm-audiologger/internal/metadata"
	"github.com/oszuidwest/zwfm-audiologger/internal/timeutil"
)

// Manager coordinates audio stream recording operations for configured stations.
type Manager struct {
	config          *config.Config
	metadataFetcher *metadata.Fetcher
	alerter         *alerting.Manager
}

// New creates a Manager with the given configuration and alert manager.
func New(cfg *config.Config, alerter *alerting.Manager) *Manager {
	return &Manager{
		config:          cfg,
		metadataFetcher: metadata.New(),
		alerter:         alerter,
	}
}

// Record performs a recording for the specified station.
// If duration is nil, records for the default hourly duration.
// If timestamp is empty, generates a timestamp automatically.
// Returns the final file path on success, or empty string on failure.
func (m *Manager) Record(ctx context.Context, name string, station *config.Station, duration *int, timestamp string) string {
	// Use provided timestamp or generate one for hourly recording
	if timestamp == "" {
		timestamp = timeutil.HourlyTimestamp()
	}

	// Fetch metadata if configured (single place for metadata logic)
	if station.MetadataURL != "" {
		go m.saveMetadata(ctx, name, station, timestamp)
	}

	// Determine duration and timeout based on whether custom duration was provided
	var durationStr string
	var timeout time.Duration

	if duration == nil {
		// Default hourly recording
		durationStr = strconv.Itoa(constants.HourlyRecordingDurationSeconds)
		timeout = constants.HourlyRecordingTimeout
	} else {
		// Custom duration (e.g., mid-hour recording)
		durationStr = strconv.Itoa(*duration)
		// Calculate timeout: duration + 5 minutes buffer for connection/processing
		timeout = time.Duration(*duration)*time.Second + 5*time.Minute
	}

	return m.record(ctx, name, station, timestamp, durationStr, timeout)
}

// record performs a recording with the given parameters.
// Returns the final file path on success, or empty string on failure.
func (m *Manager) record(ctx context.Context, name string, station *config.Station, timestamp, duration string, timeout time.Duration) string {
	dir := filepath.Join(m.config.RecordingsDir, name)
	if err := ensureDir(dir); err != nil {
		slog.Error("failed to create directory", "station", name, "error", err, "recordings_dir", m.config.RecordingsDir, "computed_dir", dir)
		return ""
	}

	// Use .mkv extension for temporary files - supports any audio codec
	tempFile := recordingPath(m.config.RecordingsDir, name, timestamp, ".mkv")

	slog.Info("Recording started", "station", name, "file", tempFile)

	// Create a context with a long timeout for recording
	recordCtx, recordCancel := context.WithTimeout(ctx, timeout)
	defer recordCancel()

	cmd := ffmpeg.RecordCommand(recordCtx, station.StreamURL, duration, tempFile)
	slog.Debug("FFmpeg args", "args", cmd.Args)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	recordCancel() // Explicitly cancel context after FFmpeg completes

	if err != nil {
		slog.Error("failed recording", "station", name, "error", err, "ffmpeg_command", strings.Join(cmd.Args[1:], " "), "stream_url", station.StreamURL, "output_file", tempFile, "ffmpeg_output", ffmpeg.TruncateOutput(output, ffmpeg.MaxOutputLogLength))

		// Alert on stream failure (alerter handles rate limiting internally)
		if m.alerter != nil {
			event := alerting.NewEvent(alerting.EventStreamOffline, name, err.Error())
			m.alerter.Alert(ctx, event)
		}

		// Clean up temp file if it was created
		cleanupTempFile(tempFile, "recording error")
		return ""
	}

	// Recording succeeded - alert recovery if station was previously failed
	if m.alerter != nil {
		event := alerting.NewEvent(alerting.EventStreamRecovered, name, "Stream has recovered and is recording normally")
		m.alerter.Alert(ctx, event)
	}

	// Detect format from the recorded file and remux to proper container
	// Use a 1-minute timeout for format detection (should be very quick)
	formatCtx, formatCancel := context.WithTimeout(ctx, 1*time.Minute)
	format := ffmpeg.DetectAudioFormat(formatCtx, tempFile)
	formatCancel()
	finalFile := recordingPath(m.config.RecordingsDir, name, timestamp, format)

	// Remux the .mkv file to proper container format
	// Use a 5-minute timeout for remuxing (should be quick with stream copy)
	remuxCtx, remuxCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer remuxCancel()

	remuxCmd := ffmpeg.RemuxCommand(remuxCtx, tempFile, finalFile)
	remuxOutput, err := remuxCmd.CombinedOutput()
	if err != nil {
		slog.Error("failed to remux recording", "station", name, "temp_file", tempFile, "final_file", finalFile, "error", err, "remux_output", ffmpeg.TruncateOutput(remuxOutput, ffmpeg.MaxOutputLogLength))

		// Clean up temp file when remux fails
		cleanupTempFile(tempFile, "remux error")
		return ""
	}

	// Remove the temporary .mkv file after successful remux
	cleanupTempFile(tempFile, "successful remux")

	slog.Info("Recording completed", "file", finalFile, "format", format)
	return finalFile
}

// saveMetadata fetches and saves metadata for a recording.
func (m *Manager) saveMetadata(ctx context.Context, stationName string, station *config.Station, timestamp string) {
	meta := m.metadataFetcher.Fetch(
		ctx,
		station.MetadataURL,
		station.MetadataPath,
		station.ParseMetadata,
	)

	if meta != "" {
		metaFile := recordingPath(m.config.RecordingsDir, stationName, timestamp, ".meta")
		if err := os.WriteFile(metaFile, []byte(meta), constants.FilePermissions); err != nil {
			slog.Error("failed to save metadata", "station", stationName, "file", metaFile, "error", err)
		} else {
			slog.Info("Saved metadata", "station", stationName, "metadata", meta)
		}
	}
}

// Test performs a test recording for all stations.
func (m *Manager) Test(ctx context.Context) {
	slog.Info("Running test recordings (10 seconds each)")

	for name, station := range m.config.Stations {
		timestamp := "test-" + timeutil.TestTimestamp()
		m.record(ctx, name, &station, timestamp, strconv.Itoa(constants.TestRecordingDurationSeconds), constants.TestRecordingTimeout)
	}

	slog.Info("Test recordings completed")
}
