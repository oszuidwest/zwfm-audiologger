// Package recorder handles audio stream recording functionality.
package recorder

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
	"github.com/oszuidwest/zwfm-audiologger/internal/metadata"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// Validator defines the interface for recording validation.
type Validator interface {
	// Enqueue adds a completed recording to the validation queue.
	Enqueue(filePath, station, timestamp string)
	// MarkSkipped writes a validation sidecar that marks a recording as valid
	// without running validation checks. Used for catchup recordings so that
	// scanUnvalidated does not re-queue them on the next startup.
	MarkSkipped(filePath, station, timestamp string)
}

// Notifier defines the interface for recording failure notifications.
type Notifier interface {
	NotifyRecordingFailure(station, reason string)
}

// Manager handles recording operations.
type Manager struct {
	config          *config.Config
	metadataFetcher *metadata.Fetcher
	validator       Validator
	notifier        Notifier
}

// New creates a new recording manager.
func New(cfg *config.Config, validator Validator, notifier Notifier) *Manager {
	return &Manager{
		config:          cfg,
		metadataFetcher: metadata.New(),
		validator:       validator,
		notifier:        notifier,
	}
}

// Scheduled performs a scheduled recording with 1 hour duration.
func (m *Manager) Scheduled(name string, station *config.Station) {
	timestamp := utils.HourlyTimestamp()

	// Fetch metadata if configured
	if station.MetadataURL != "" {
		go m.saveMetadata(name, station, timestamp)
	}

	m.record(name, station, timestamp, constants.HourlyRecordingDuration, constants.HourlyRecordingTimeout, false)
}

// Catchup performs a recording for the remainder of the current hour after a mid-hour startup.
func (m *Manager) Catchup(name string, station *config.Station, timestamp string, durationSecs int) {
	duration := strconv.Itoa(durationSecs)
	timeout := time.Duration(durationSecs+300) * time.Second // 5-minute buffer beyond duration

	if station.MetadataURL != "" {
		go m.saveMetadata(name, station, timestamp)
	}

	// Skip validation: catchup recordings are partial by definition and would
	// always fail the MinDurationSecs check.
	m.record(name, station, timestamp, duration, timeout, true)
}

// record performs the actual recording operation.
func (m *Manager) record(name string, station *config.Station, timestamp, duration string, timeout time.Duration, skipValidation bool) {
	dir := filepath.Join(m.config.RecordingsDir, name)
	if err := utils.EnsureDir(dir); err != nil {
		reason := fmt.Sprintf("failed to create recording directory: %v", err)
		slog.Error("skipping recording", "station", name, "reason", reason, "recordings_dir", m.config.RecordingsDir, "computed_dir", dir)
		if m.notifier != nil {
			m.notifier.NotifyRecordingFailure(name, reason)
		}
		return
	}

	// Refuse to record if available disk space is below the minimum threshold.
	available, err := utils.AvailableDiskBytes(dir)
	if err != nil {
		reason := fmt.Sprintf("disk space check failed: %v", err)
		slog.Error("skipping recording", "station", name, "reason", reason)
		if m.notifier != nil {
			m.notifier.NotifyRecordingFailure(name, reason)
		}
		return
	}
	if available < constants.MinDiskSpaceBytes {
		reason := fmt.Sprintf("insufficient disk space: %d bytes available, %d required", available, constants.MinDiskSpaceBytes)
		slog.Error("skipping recording", "station", name, "reason", reason)
		if m.notifier != nil {
			m.notifier.NotifyRecordingFailure(name, reason)
		}
		return
	}

	// Use .mkv extension for temporary files - supports any audio codec
	tempFile := utils.RecordingPath(m.config.RecordingsDir, name, timestamp, ".mkv")

	slog.Info("Recording started", "station", name, "file", tempFile)

	// Create a context with a long timeout for recording
	recordCtx, recordCancel := context.WithTimeout(context.Background(), timeout)
	defer recordCancel()

	cmd := utils.RecordCommand(recordCtx, station.StreamURL, duration, tempFile)
	slog.Debug("FFmpeg args", "args", cmd.Args)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	recordCancel() // Explicitly cancel context after FFmpeg completes

	if err != nil {
		// Limit output to first 500 bytes to avoid excessive logging
		outputStr := string(output)
		if len(outputStr) > 500 {
			outputStr = outputStr[:500] + "... (truncated)"
		}
		slog.Error("failed recording", "station", name, "error", err, "ffmpeg_command", strings.Join(cmd.Args[1:], " "), "stream_url", station.StreamURL, "output_file", tempFile, "ffmpeg_output", outputStr)

		if m.notifier != nil {
			m.notifier.NotifyRecordingFailure(name, fmt.Sprintf("FFmpeg failed: %v", err))
		}

		// Clean up temp file if it was created
		if err := os.Remove(tempFile); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to clean up temp file after error", "file", tempFile, "error", err)
		}
		return
	}

	// Detect format from the recorded file and remux to proper container
	format := utils.Format(tempFile)
	finalFile := utils.RecordingPath(m.config.RecordingsDir, name, timestamp, format)

	// Remux the .mkv file to proper container format
	remuxCmd := utils.RemuxCommand(tempFile, finalFile)
	remuxOutput, err := remuxCmd.CombinedOutput()
	if err != nil {
		// Limit output to first 500 bytes to avoid excessive logging
		outputStr := string(remuxOutput)
		if len(outputStr) > 500 {
			outputStr = outputStr[:500] + "... (truncated)"
		}
		slog.Error("failed to remux recording", "station", name, "temp_file", tempFile, "final_file", finalFile, "error", err, "remux_output", outputStr)

		if m.notifier != nil {
			m.notifier.NotifyRecordingFailure(name, fmt.Sprintf("remux failed: %v", err))
		}

		// Clean up temp file when remux fails
		if err := os.Remove(tempFile); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to clean up temp file after remux error", "file", tempFile, "error", err)
		}
		return
	}

	// Remove the temporary .mkv file after successful remux
	if err := os.Remove(tempFile); err != nil {
		slog.Warn("failed to remove temporary file", "file", tempFile, "error", err)
	}

	slog.Info("Recording completed", "file", finalFile, "format", format)

	// For full recordings, enqueue for validation. For catchup recordings, write a
	// sidecar immediately so scanUnvalidated does not re-queue the file on restart.
	if m.validator != nil {
		if skipValidation {
			m.validator.MarkSkipped(finalFile, name, timestamp)
		} else {
			m.validator.Enqueue(finalFile, name, timestamp)
		}
	}
}

// saveMetadata fetches and saves metadata for a recording.
func (m *Manager) saveMetadata(stationName string, station *config.Station, timestamp string) {
	meta := m.metadataFetcher.Fetch(
		station.MetadataURL,
		station.MetadataPath,
		station.ParseMetadata,
	)

	if meta != "" {
		metaFile := utils.RecordingPath(m.config.RecordingsDir, stationName, timestamp, ".meta")
		if err := os.WriteFile(metaFile, []byte(meta), constants.FilePermissions); err != nil {
			slog.Error("failed to save metadata", "station", stationName, "file", metaFile, "error", err)
		} else {
			slog.Info("Saved metadata", "station", stationName, "metadata", meta)
		}
	}
}

// Test performs a test recording for all stations.
func (m *Manager) Test() {
	slog.Info("Running test recordings (10 seconds each)")

	for name, station := range m.config.Stations {
		timestamp := "test-" + utils.TestTimestamp()
		m.record(name, &station, timestamp, constants.TestRecordingDuration, constants.TestRecordingTimeout, false)
	}

	slog.Info("Test recordings completed")
}
