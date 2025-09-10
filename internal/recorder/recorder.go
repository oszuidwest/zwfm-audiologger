// Package recorder handles audio stream recording functionality
package recorder

import (
	"context"
	"log"
	"os"

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
	if err := utils.EnsureDir(dir); err != nil {
		log.Printf("Failed to create directory for %s: %v", name, err)
		return
	}

	timestamp := utils.HourlyTimestamp()
	// Record with temporary .rec extension
	tempFile := utils.RecordingPath(m.config.RecordingsDir, name, timestamp, ".rec")

	// Fetch metadata if configured
	if station.MetadataURL != "" {
		go m.saveMetadata(name, station, timestamp)
	}

	log.Printf("Recording %s to %s", name, tempFile)

	cmd := utils.RecordCommand(context.Background(), station.StreamURL, "3600", tempFile)

	if err := cmd.Run(); err != nil {
		log.Printf("Failed recording for %s: %v", name, err)
		return
	}

	// Detect format from the recorded file and rename
	format := utils.Format(tempFile)
	finalFile := utils.RecordingPath(m.config.RecordingsDir, name, timestamp, format)

	if err := os.Rename(tempFile, finalFile); err != nil {
		log.Printf("Failed to rename recording: %v", err)
		return
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
		// Record with temporary .rec extension
		tempFile := utils.RecordingPath(m.config.RecordingsDir, name, "test-"+timestamp, ".rec")

		log.Printf("Test recording %s to %s", name, tempFile)

		cmd := utils.RecordCommand(ctx, station.StreamURL, "10", tempFile)

		if err := cmd.Run(); err != nil {
			log.Printf("Failed test recording for %s: %v", name, err)
			continue
		}

		// Detect format and rename
		format := utils.Format(tempFile)
		finalFile := utils.RecordingPath(m.config.RecordingsDir, name, "test-"+timestamp, format)

		if err := os.Rename(tempFile, finalFile); err != nil {
			log.Printf("Failed to rename test recording: %v", err)
			continue
		}

		if info, err := os.Stat(finalFile); err == nil {
			log.Printf("Test recording completed: %s (size: %d bytes, format: %s)", finalFile, info.Size(), format)
		}
	}

	log.Println("Test recordings completed.")
}
