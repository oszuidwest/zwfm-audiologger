// Package postprocessor handles trimming commercials from recordings
package postprocessor

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	recordings    map[string]*Recording // Key: "station-hour"
	mu            sync.RWMutex
}

// New creates a new post-processor manager
func New(recordingsDir string) *Manager {
	return &Manager{
		recordingsDir: recordingsDir,
		recordings:    make(map[string]*Recording),
	}
}

// MarkProgramStart marks when a program starts (commercials end)
func (m *Manager) MarkProgramStart(station string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	hour := now.Format(utils.HourlyTimestampFormat)
	key := fmt.Sprintf("%s-%s", station, hour)

	// Create or update recording with start time
	if m.recordings[key] == nil {
		m.recordings[key] = &Recording{
			StartTime: now,
			Station:   station,
			Hour:      hour,
		}
	} else {
		// Update start time if recording already exists
		m.recordings[key].StartTime = now
	}

	log.Printf("Program started for %s at %s", station, now.Format("15:04:05"))

	// Save recording info to file for persistence
	m.saveRecordingInfo(station, hour)
}

// MarkProgramEnd marks when a program ends (commercials start)
func (m *Manager) MarkProgramEnd(station string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	hour := now.Format(utils.HourlyTimestampFormat)
	key := fmt.Sprintf("%s-%s", station, hour)

	// Update the recording's end time
	if recording, exists := m.recordings[key]; exists {
		recording.EndTime = now

		log.Printf("Program ended for %s at %s", station, now.Format("15:04:05"))

		// Save recording info to file for persistence
		m.saveRecordingInfo(station, hour)
	}
}

// Recording processes a completed hourly recording to remove commercials
func (m *Manager) Recording(station, hour string) error {
	m.mu.RLock()
	key := fmt.Sprintf("%s-%s", station, hour)
	recording, exists := m.recordings[key]
	m.mu.RUnlock()

	if !exists || recording == nil {
		log.Printf("No recording info found for %s hour %s, keeping full recording", station, hour)
		return nil
	}

	// Find the input file with any audio extension
	inputFile := utils.FindRecordingFile(m.recordingsDir, station, hour)
	if inputFile == "" {
		return fmt.Errorf("recording file not found for %s hour %s", station, hour)
	}

	// Keep original as backup, processed will take its name
	ext := utils.GetFileExtension(inputFile)
	originalBackup := utils.BuildRecordingPath(m.recordingsDir, station, hour+".original", ext)
	tempOutput := utils.BuildRecordingPath(m.recordingsDir, station, hour+".temp", ext)

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
	cmd := utils.FFmpegTrimCommand(inputFile, fmt.Sprintf("%.0f", startOffset), fmt.Sprintf("%.0f", duration), tempOutput)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg trim failed: %w", err)
	}

	// Rename original to .original backup
	if err := os.Rename(inputFile, originalBackup); err != nil {
		os.Remove(tempOutput) // Clean up temp file
		return fmt.Errorf("failed to backup original: %w", err)
	}

	// Rename processed to original name for predictable URLs
	if err := os.Rename(tempOutput, inputFile); err != nil {
		// Try to restore original if rename fails
		os.Rename(originalBackup, inputFile)
		os.Remove(tempOutput)
		return fmt.Errorf("failed to replace with processed version: %w", err)
	}

	log.Printf("Processed recording: %s (original backed up as %s)", inputFile, originalBackup)

	// Clean up recording data after processing
	m.mu.Lock()
	delete(m.recordings, key)
	m.mu.Unlock()

	return nil
}

// saveRecordingInfo saves recording information to a JSON file for persistence
func (m *Manager) saveRecordingInfo(station, hour string) {
	key := fmt.Sprintf("%s-%s", station, hour)
	recording := m.recordings[key]

	if recording == nil {
		return
	}

	// Save to a JSON file alongside the recording
	recordingFile := utils.BuildRecordingPath(m.recordingsDir, station, hour, ".recording.json")

	data, err := json.MarshalIndent(recording, "", "  ")
	if err != nil {
		utils.LogErrorAndContinue("marshal recording", err)
		return
	}

	if err := os.WriteFile(recordingFile, data, 0644); err != nil {
		utils.LogErrorAndContinue("save recording info", err)
		return
	}

	log.Printf("Saved recording info to %s", recordingFile)
}

// LoadInfo loads saved recording information from disk
func (m *Manager) LoadInfo(station, hour string) error {
	recordingFile := utils.BuildRecordingPath(m.recordingsDir, station, hour, ".recording.json")

	data, err := os.ReadFile(recordingFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No recording file is fine
		}
		return err
	}

	var recording Recording
	if err := json.Unmarshal(data, &recording); err != nil {
		return err
	}

	m.mu.Lock()
	key := fmt.Sprintf("%s-%s", station, hour)
	m.recordings[key] = &recording
	m.mu.Unlock()

	log.Printf("Loaded recording info for %s hour %s", station, hour)
	return nil
}

// PendingRecordings processes any recordings that have recording info but haven't been processed yet
func (m *Manager) PendingRecordings() error {
	// Look for .recording.json files without corresponding _processed.mp3 files
	stations, err := os.ReadDir(m.recordingsDir)
	if err != nil {
		return err
	}

	for _, stationDir := range stations {
		if !stationDir.IsDir() {
			continue
		}

		stationPath := utils.BuildStationDir(m.recordingsDir, stationDir.Name())
		files, err := os.ReadDir(stationPath)
		if err != nil {
			continue
		}

		for _, file := range files {
			if filepath.Ext(file.Name()) == ".json" && strings.Contains(file.Name(), ".recording.json") {
				hour := strings.TrimSuffix(file.Name(), ".recording.json")

				// Check if we have an original backup (means it was already processed)
				originalFiles, _ := filepath.Glob(filepath.Join(stationPath, hour+".original.*"))
				if len(originalFiles) > 0 {
					continue // Already processed
				}

				// Find the actual recording file
				recordingFile := utils.FindRecordingFile(m.recordingsDir, stationDir.Name(), hour)
				if recordingFile == "" {
					continue // No recording file found
				}

				// Load recording info and process
				if err := m.LoadInfo(stationDir.Name(), hour); err == nil {
					log.Printf("Processing pending recording: %s %s", stationDir.Name(), hour)
					m.Recording(stationDir.Name(), hour)
				}
			}
		}
	}

	return nil
}
