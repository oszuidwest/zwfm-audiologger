// Package recorder handles audio stream recording functionality
package recorder

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/config"
	"github.com/oszuidwest/zwfm-audiologger/internal/metadata"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// Manager handles recording operations
type Manager struct {
	config          *config.Config
	metadataFetcher *metadata.Fetcher
}

// New creates a new recording manager
func New(cfg *config.Config) *Manager {
	return &Manager{
		config:          cfg,
		metadataFetcher: metadata.New(),
	}
}

// Scheduled performs a scheduled recording (1 hour duration)
func (m *Manager) Scheduled(name string, station config.Station) {
	dir := utils.StationDir(m.config.RecordingsDir, name)
	log.Printf("DEBUG: Ensuring directory exists: %s", dir)
	if err := utils.EnsureDir(dir); err != nil {
		log.Printf("CRITICAL: Failed to create directory for %s: %v", name, err)
		log.Printf("DEBUG: RecordingsDir=%s, Station=%s, ComputedDir=%s", m.config.RecordingsDir, name, dir)
		return
	}
	log.Printf("DEBUG: Directory creation successful: %s", dir)

	timestamp := utils.HourlyTimestamp()
	// Use .mkv extension for temporary files - supports any audio codec
	tempFile := utils.RecordingPath(m.config.RecordingsDir, name, timestamp, ".mkv")

	// Fetch metadata if configured
	if station.MetadataURL != "" {
		go m.saveMetadata(name, station, timestamp)
	}

	log.Printf("Recording %s to %s", name, tempFile)
	log.Printf("DEBUG: Full FFmpeg command about to execute...")

	// Create a context with a long timeout for recording
	recordCtx, recordCancel := context.WithTimeout(context.Background(), 65*time.Minute) // Slightly longer than recording duration
	defer recordCancel()

	cmd := utils.RecordCommand(recordCtx, station.StreamURL, "3600", tempFile)
	log.Printf("DEBUG: FFmpeg args: %v", cmd.Args)

	log.Printf("DEBUG: Starting FFmpeg process...")

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("CRITICAL: Failed recording for %s: %v", name, err)
		log.Printf("DEBUG: FFmpeg command: ffmpeg %s", strings.Join(cmd.Args[1:], " "))
		log.Printf("DEBUG: Stream URL: %s", station.StreamURL)
		log.Printf("DEBUG: Output file path: %s", tempFile)
		log.Printf("DEBUG: FFmpeg output: %s", string(output))

		// Check if output file was created despite error
		if info, statErr := os.Stat(tempFile); statErr == nil {
			log.Printf("DEBUG: Output file WAS created despite error, size: %d bytes", info.Size())
		} else {
			log.Printf("DEBUG: Output file was NOT created: %v", statErr)
		}

		// Check parent directory permissions
		if dirInfo, dirErr := os.Stat(dir); dirErr == nil {
			log.Printf("DEBUG: Parent directory exists, permissions: %v", dirInfo.Mode())
		} else {
			log.Printf("DEBUG: Parent directory check failed: %v", dirErr)
		}
		return
	}
	log.Printf("DEBUG: FFmpeg process completed successfully")
	log.Printf("DEBUG: FFmpeg output: %s", string(output))

	// Detect format from the recorded file and remux to proper container
	format := utils.Format(tempFile)
	finalFile := utils.RecordingPath(m.config.RecordingsDir, name, timestamp, format)

	// Remux the .rec file to proper container format
	remuxCmd := utils.RemuxCommand(tempFile, finalFile)
	remuxOutput, err := remuxCmd.CombinedOutput()
	if err != nil {
		log.Printf("Failed to remux recording: %v", err)
		log.Printf("DEBUG: Remux output: %s", string(remuxOutput))
		return
	}

	// Remove the temporary .rec file after successful remux
	if err := os.Remove(tempFile); err != nil {
		log.Printf("Warning: Failed to remove temporary file %s: %v", tempFile, err)
	}

	log.Printf("Recording completed: %s (format: %s)", finalFile, format)
}

// saveMetadata fetches and saves metadata for a recording
func (m *Manager) saveMetadata(stationName string, station config.Station, timestamp string) {
	meta := m.metadataFetcher.Fetch(
		station.MetadataURL,
		station.MetadataPath,
		station.ParseMetadata,
	)

	if meta != "" {
		metaFile := utils.RecordingPath(m.config.RecordingsDir, stationName, timestamp, ".meta")
		if err := os.WriteFile(metaFile, []byte(meta), 0o644); err != nil {
			log.Printf("Failed to save metadata for %s: %v", stationName, err)
		} else {
			log.Printf("Saved metadata for %s: %s", stationName, meta)
		}
	}
}

// Test performs a test recording for all stations
func (m *Manager) Test(ctx context.Context) {
	log.Println("Running test recordings (10 seconds each)...")

	for name, station := range m.config.Stations {
		dir := utils.StationDir(m.config.RecordingsDir, name)
		if err := utils.EnsureDir(dir); err != nil {
			log.Printf("Failed to create directory for %s: %v", name, err)
			continue
		}

		timestamp := utils.TestTimestamp()
		// Record with temporary .mkv extension
		tempFile := utils.RecordingPath(m.config.RecordingsDir, name, "test-"+timestamp, ".mkv")

		log.Printf("Test recording %s to %s", name, tempFile)

		// Create dedicated context for test recording to avoid cancellation issues
		testCtx, testCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer testCancel()

		cmd := utils.RecordCommand(testCtx, station.StreamURL, "10", tempFile)

		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("Failed test recording for %s: %v", name, err)
			log.Printf("DEBUG: FFmpeg test output: %s", string(output))
			continue
		}

		// Detect format and remux to proper container
		format := utils.Format(tempFile)
		finalFile := utils.RecordingPath(m.config.RecordingsDir, name, "test-"+timestamp, format)

		// Remux the .rec file to proper container format
		remuxCmd := utils.RemuxCommand(tempFile, finalFile)
		remuxOutput, err := remuxCmd.CombinedOutput()
		if err != nil {
			log.Printf("Failed to remux test recording: %v", err)
			log.Printf("DEBUG: Remux output: %s", string(remuxOutput))
			continue
		}

		// Remove the temporary .rec file after successful remux
		if err := os.Remove(tempFile); err != nil {
			log.Printf("Warning: Failed to remove temporary test file %s: %v", tempFile, err)
		}

		if info, err := os.Stat(finalFile); err == nil {
			log.Printf("Test recording completed: %s (size: %d bytes, format: %s)", finalFile, info.Size(), format)
		}
	}

	log.Println("Test recordings completed.")
}
