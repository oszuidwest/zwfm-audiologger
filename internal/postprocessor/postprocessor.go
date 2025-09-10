// Package postprocessor handles trimming commercials from recordings
package postprocessor

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oszuidwest/zwfm-audiologger/internal/utils"
)

// Recording represents a program recording within an hour
type Recording struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Station   string    `json:"station"`
	Hour      string    `json:"hour"` // Format: "2006-01-02-15"
}

// Manager handles post-processing of recordings
type Manager struct {
	recordingsDir string
}

// New creates a new post-processor manager
func New(recordingsDir string) *Manager {
	return &Manager{
		recordingsDir: recordingsDir,
	}
}

// MarkProgramStart marks when a program starts (commercials end)
func (m *Manager) MarkProgramStart(station string) {
	now := utils.Now()
	hour := now.Format(utils.HourlyTimestampFormat)

	// Load existing recording or create new one
	recording := m.loadRecording(station, hour)
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

	log.Printf("Program started for %s at %s", station, now.Format("15:04:05"))

	// Save recording info to file
	m.saveRecording(recording)
}

// MarkProgramEnd marks when a program ends (commercials start)
func (m *Manager) MarkProgramEnd(station string) {
	now := utils.Now()
	hour := now.Format(utils.HourlyTimestampFormat)

	// Load existing recording
	recording := m.loadRecording(station, hour)
	if recording != nil {
		recording.EndTime = now
		log.Printf("Program ended for %s at %s", station, now.Format("15:04:05"))
		// Save recording info to file
		m.saveRecording(recording)
	}
}

// ProcessRecording processes a completed hourly recording to remove commercials
func (m *Manager) ProcessRecording(station, hour string) error {
	recording := m.loadRecording(station, hour)
	if recording == nil {
		log.Printf("No recording info found for %s hour %s, keeping full recording", station, hour)
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

	log.Printf("Trimming %s: extracting from %v for %v seconds", station, startOffset, duration)

	// Process to temporary file
	cmd := utils.TrimCommand(inputFile, fmt.Sprintf("%.0f", startOffset), fmt.Sprintf("%.0f", duration), tempOutput)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg trim failed: %w", err)
	}

	// Rename original to .original backup
	if err := os.Rename(inputFile, originalBackup); err != nil {
		if removeErr := os.Remove(tempOutput); removeErr != nil {
			log.Printf("Failed to clean up temp file %s: %v", tempOutput, removeErr)
		}
		return fmt.Errorf("failed to backup original: %w", err)
	}

	// Rename processed to original name for predictable URLs
	if err := os.Rename(tempOutput, inputFile); err != nil {
		// Try to restore original if rename fails
		if restoreErr := os.Rename(originalBackup, inputFile); restoreErr != nil {
			log.Printf("Failed to restore original file %s: %v", inputFile, restoreErr)
		}
		if removeErr := os.Remove(tempOutput); removeErr != nil {
			log.Printf("Failed to remove temp file %s: %v", tempOutput, removeErr)
		}
		return fmt.Errorf("failed to replace with processed version: %w", err)
	}

	log.Printf("Processed recording: %s (original backed up as %s)", inputFile, originalBackup)

	// Processing complete - JSON file remains for reference

	return nil
}

// saveRecording saves recording information to a JSON file
func (m *Manager) saveRecording(recording *Recording) {
	if recording == nil {
		return
	}

	// Save to a JSON file alongside the recording
	recordingFile := utils.RecordingPath(m.recordingsDir, recording.Station, recording.Hour, ".recording.json")

	data, err := json.MarshalIndent(recording, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal recording: %v", err)
		return
	}

	if err := os.WriteFile(recordingFile, data, 0o644); err != nil {
		log.Printf("Failed to save recording info: %v", err)
		return
	}

	log.Printf("Saved recording info to %s", recordingFile)
}

// loadRecording loads recording information from disk
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

// ProcessPendingRecordings processes any recordings that have recording info but haven't been processed yet
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

		stationPath := utils.StationDir(m.recordingsDir, stationDir.Name())
		files, err := os.ReadDir(stationPath)
		if err != nil {
			continue
		}

		for _, file := range files {
			if filepath.Ext(file.Name()) != ".json" || !strings.Contains(file.Name(), ".recording.json") {
				continue
			}

			hour := strings.TrimSuffix(file.Name(), ".recording.json")

			// Check if we have an original backup (means it was already processed)
			originalFiles, err := filepath.Glob(filepath.Join(stationPath, hour+".original.*"))
			if err != nil {
				log.Printf("Invalid glob pattern for %s: %v", hour, err)
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
			log.Printf("Processing pending recording: %s %s", stationDir.Name(), hour)
			if err := m.ProcessRecording(stationDir.Name(), hour); err != nil {
				log.Printf("Failed to process pending recording %s %s: %v", stationDir.Name(), hour, err)
			}
		}
	}

	return nil
}
