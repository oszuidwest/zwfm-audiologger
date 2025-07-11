// Package postprocessor handles trimming commercials from recordings.
package postprocessor

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/constants"
	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// Recording represents a program recording within an hour.
type Recording struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Station   string    `json:"station"`
	Hour      string    `json:"hour"` // Format: "2006-01-02-15"
}

// Manager handles post-processing of recordings.
type Manager struct {
	recordingsDir string
	mu            sync.RWMutex
}

// New creates a new post-processor manager.
func New(recordingsDir string) *Manager {
	return &Manager{
		recordingsDir: recordingsDir,
	}
}

// MarkType represents the type of program mark.
type MarkType int

const (
	// MarkStart indicates program start (commercials end).
	MarkStart MarkType = iota
	// MarkEnd indicates program end (commercials start).
	MarkEnd
)

// MarkProgram marks when a program starts or ends.
func (m *Manager) MarkProgram(station string, markType MarkType) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := utils.Now()
	hour := now.Format(utils.HourlyTimestampFormat)

	// Load existing recording or create new one for MarkStart
	recording := m.loadRecording(station, hour)

	switch markType {
	case MarkStart:
		if recording == nil {
			recording = &Recording{
				StartTime: now,
				Station:   station,
				Hour:      hour,
			}
		} else {
			// Update start time if recording already exists
			recording.StartTime = now
		}
		slog.Info("Program started", "station", station, "time", now.Format("15:04:05"))
		m.saveRecording(recording)

	case MarkEnd:
		if recording != nil {
			recording.EndTime = now
			slog.Info("Program ended", "station", station, "time", now.Format("15:04:05"))
			m.saveRecording(recording)
		}
	}
}

// ProcessRecording processes a completed hourly recording to remove commercials.
func (m *Manager) ProcessRecording(station, hour string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	recording := m.loadRecording(station, hour)
	if recording == nil {
		slog.Info("No recording info found, keeping full recording", "station", station, "hour", hour)
		return nil
	}

	// Find the input file with any audio extension
	inputFile, err := utils.FindRecordingFile(m.recordingsDir, station, hour)
	if err != nil {
		return fmt.Errorf("recording file not found for %s hour %s: %w", station, hour, err)
	}

	// Keep original as backup, processed will take its name
	ext := utils.Extension(inputFile)
	originalBackup := utils.RecordingPath(m.recordingsDir, station, hour+".original", ext)
	tempOutput := utils.RecordingPath(m.recordingsDir, station, hour+".temp", ext)

	// Parse the hour to get the recording start time
	recordingStart, err := utils.ParseHourlyTimestamp(hour)
	if err != nil {
		return fmt.Errorf("invalid hour format: %s", hour)
	}

	// Calculate offsets
	startOffset := recording.StartTime.Sub(recordingStart).Seconds()
	if startOffset < 0 {
		startOffset = 0
	}

	var duration float64
	if !recording.EndTime.IsZero() {
		duration = recording.EndTime.Sub(recording.StartTime).Seconds()
	} else {
		// If no end time, go to the end of the recording
		duration = 3600 - startOffset // Assuming 1-hour recordings
	}

	slog.Info("Trimming recording", "station", station, "start_offset", startOffset, "duration", duration)

	// Process to temporary file
	cmd := utils.TrimCommand(inputFile, fmt.Sprintf("%.0f", startOffset), fmt.Sprintf("%.0f", duration), tempOutput)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg trim failed for %s: %w", inputFile, err)
	}

	// Rename original to .original backup
	if err := os.Rename(inputFile, originalBackup); err != nil {
		if removeErr := os.Remove(tempOutput); removeErr != nil {
			slog.Warn("failed to clean up temp file", "file", tempOutput, "error", removeErr)
		}
		return fmt.Errorf("failed to backup original %s: %w", inputFile, err)
	}

	// Rename processed to original name for predictable URLs
	if err := os.Rename(tempOutput, inputFile); err != nil {
		// Try to restore original if rename fails
		if restoreErr := os.Rename(originalBackup, inputFile); restoreErr != nil {
			slog.Error("failed to restore original file", "file", inputFile, "error", restoreErr)
		}
		if removeErr := os.Remove(tempOutput); removeErr != nil {
			slog.Warn("failed to remove temp file", "file", tempOutput, "error", removeErr)
		}
		return fmt.Errorf("failed to replace %s with processed version: %w", inputFile, err)
	}

	slog.Info("Processed recording", "file", inputFile, "backup", originalBackup)

	// Processing complete - JSON file remains for reference

	return nil
}

// saveRecording saves recording information to a JSON file.
// Callers must ensure recording is not nil before calling this function.
func (m *Manager) saveRecording(recording *Recording) {
	// Save to a JSON file alongside the recording
	recordingFile := utils.RecordingPath(m.recordingsDir, recording.Station, recording.Hour, ".recording.json")

	data, err := json.MarshalIndent(recording, "", "  ")
	if err != nil {
		slog.Error("failed to marshal recording", "station", recording.Station, "error", err)
		return
	}

	if err := os.WriteFile(recordingFile, data, constants.FilePermissions); err != nil {
		slog.Error("failed to save recording info", "file", recordingFile, "error", err)
		return
	}

	slog.Info("Saved recording info", "file", recordingFile)
}

// loadRecording loads recording information from disk.
func (m *Manager) loadRecording(station, hour string) *Recording {
	recordingFile := utils.RecordingPath(m.recordingsDir, station, hour, ".recording.json")

	data, err := os.ReadFile(recordingFile)
	if err != nil {
		return nil // No recording file
	}

	var recording Recording
	if err := json.Unmarshal(data, &recording); err != nil {
		return nil
	}

	return &recording
}

// ProcessPendingRecordings processes any recordings that have recording info but haven't been processed yet.
func (m *Manager) ProcessPendingRecordings() error {
	// Look for .recording.json files without corresponding _processed.mp3 files
	stations, err := os.ReadDir(m.recordingsDir)
	if err != nil {
		return err
	}

	for _, stationDir := range stations {
		if !stationDir.IsDir() {
			continue
		}

		stationPath := filepath.Join(m.recordingsDir, stationDir.Name())
		files, err := os.ReadDir(stationPath)
		if err != nil {
			continue
		}

		for _, file := range files {
			if !strings.HasSuffix(file.Name(), ".recording.json") {
				continue
			}

			hour := strings.TrimSuffix(file.Name(), ".recording.json")

			// Check if we have an original backup (means it was already processed)
			originalFiles, err := filepath.Glob(filepath.Join(stationPath, hour+".original.*"))
			if err != nil {
				slog.Error("invalid glob pattern", "hour", hour, "error", err)
				continue
			}
			if len(originalFiles) > 0 {
				continue // Already processed
			}

			// Find the actual recording file
			if _, err := utils.FindRecordingFile(m.recordingsDir, stationDir.Name(), hour); err != nil {
				continue // No recording file found
			}

			// Process recording (it will load the info directly)
			slog.Info("Processing pending recording", "station", stationDir.Name(), "hour", hour)
			if err := m.ProcessRecording(stationDir.Name(), hour); err != nil {
				slog.Error("failed to process pending recording", "station", stationDir.Name(), "hour", hour, "error", err)
			}
		}
	}

	return nil
}
